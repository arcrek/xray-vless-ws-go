package xraycore

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TestEngineTrafficReflectsRealConnection is the one test in this phase that
// touches the real app/dispatcher counter-naming/wiring this whole phase
// depends on (plan Phase 1, Red Team finding #5): config_test.go and
// supervisor_test.go alone only check JSON shape and callback plumbing,
// neither of which would catch a future xray-core version silently renaming
// the "user>>>email>>>traffic>>>..." counter format. This test builds a real
// minimal VLESS+WS inbound via BuildConfig, starts a real Engine, drives one
// actual authenticated VLESS connection through it end-to-end (a local TCP
// echo server as the freedom outbound's target), and asserts Traffic()
// reports ok=true with non-zero uplink and downlink.
func TestEngineTrafficReflectsRealConnection(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	testUUID := "b831381d-6324-4d53-ad4f-8cda48b30811"

	inboundPort := freeTestPort(t)
	ports, err := config.ParsePorts(fmt.Sprintf("127.0.0.1:%d", inboundPort))
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	cfg := &config.Config{
		Ports:    ports,
		XrayUUID: testUUID,
		WSPath:   "/statstest",
	}

	cfgBytes, err := BuildConfig(cfg)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	cfgBytes = allowLoopbackOutbound(t, cfgBytes)

	engine, err := New(cfgBytes, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer engine.Close()

	if up, down, ok := engine.Traffic(); !ok || up != 0 || down != 0 {
		t.Fatalf("Traffic() before any connection = (%d, %d, %v), want (0, 0, true)", up, down, ok)
	}

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d%s", inboundPort, cfg.WSPath)
	var conn *websocket.Conn
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, _, err = websocket.DefaultDialer.Dial(wsURL, nil)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dialing inbound WS listener: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer conn.Close()

	if err := sendVLESSRequest(conn, testUUID, echoAddr, []byte("hello xray")); err != nil {
		t.Fatalf("sending VLESS request: %v", err)
	}

	echoed, err := readVLESSResponseBody(conn, len("hello xray"))
	if err != nil {
		t.Fatalf("reading VLESS response: %v", err)
	}
	if string(echoed) != "hello xray" {
		t.Fatalf("echoed body = %q, want %q", echoed, "hello xray")
	}
	conn.Close()

	// The counters update asynchronously with the dispatcher tearing down
	// the connection — give it a brief window rather than asserting
	// immediately after Write/Read return.
	deadline = time.Now().Add(2 * time.Second)
	for {
		up, down, ok := engine.Traffic()
		if ok && up > 0 && down > 0 {
			return // success
		}
		if time.Now().After(deadline) {
			t.Fatalf("Traffic() after a real connection = (%d, %d, %v), want ok=true with non-zero uplink and downlink", up, down, ok)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// allowLoopbackOutbound patches BuildConfig's freedom outbound with a
// "finalRules": [{"action":"allow"}] override, test-only. Discovered during
// implementation (not anticipated in the plan): this pinned xray-core
// fork's freedom outbound defaults to blackholing any private/loopback
// destination from a "vless" inbound as SSRF hardening
// (proxy/freedom/freedom.go's defaultBlockPrivateRule) — real deployments
// never hit this (they proxy to real internet targets), but this test's
// local echo-server target does, so it needs an explicit allow override.
// Production BuildConfig itself is deliberately left unpatched — weakening
// that protection there would be a real behavior change, not a test
// affordance.
func allowLoopbackOutbound(t *testing.T, cfgBytes []byte) []byte {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(cfgBytes, &decoded); err != nil {
		t.Fatalf("allowLoopbackOutbound: decoding config: %v", err)
	}
	outbounds := decoded["outbounds"].([]any)
	outbound := outbounds[0].(map[string]any)
	settings := outbound["settings"].(map[string]any)
	settings["finalRules"] = []map[string]any{{"action": "allow"}}

	patched, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("allowLoopbackOutbound: re-encoding config: %v", err)
	}
	return patched
}

// startEchoServer runs a plain TCP echo listener (io.Copy(conn, conn) per
// accepted connection) that the VLESS request below targets via the
// freedom outbound — this is what actually generates real, non-zero
// uplink+downlink bytes for the counters to record.
func startEchoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting echo server: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// freeTestPort asks the OS for a free loopback TCP port, the same
// bind-then-release trick tunnel.freeLocalPort uses (not reused directly
// across packages to avoid a cross-package coupling for one helper).
func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("picking a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// sendVLESSRequest writes one minimal VLESS TCP request (version 0, no
// addons, command=TCP, IPv4 destination) followed immediately by payload,
// over an already-established WS connection — enough to authenticate as
// clientUUID and have xray-core's freedom outbound dial destAddr.
func sendVLESSRequest(conn *websocket.Conn, clientUUID, destAddr string, payload []byte) error {
	id, err := uuid.Parse(clientUUID)
	if err != nil {
		return fmt.Errorf("parsing test UUID: %w", err)
	}
	host, portStr, err := net.SplitHostPort(destAddr)
	if err != nil {
		return fmt.Errorf("splitting echo addr: %w", err)
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return fmt.Errorf("echo addr host %q is not an IPv4 literal", host)
	}
	var port uint16
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("parsing echo port: %w", err)
	}

	buf := make([]byte, 0, 24+len(payload))
	buf = append(buf, 0x00) // version
	idBytes := id[:]        // 16 raw bytes
	buf = append(buf, idBytes...)
	buf = append(buf, 0x00) // addons length: 0
	buf = append(buf, 0x01) // command: TCP
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	buf = append(buf, portBytes...)
	buf = append(buf, 0x01) // address type: IPv4
	buf = append(buf, ip...)
	buf = append(buf, payload...)

	return conn.WriteMessage(websocket.BinaryMessage, buf)
}

// readVLESSResponseBody reads VLESS response frames (version + addons-length
// + addons, discarded) until wantLen bytes of body have been collected,
// buffering across multiple WS messages since xray-core's websocket
// transport doesn't guarantee one WS message == one VLESS protocol unit.
func readVLESSResponseBody(conn *websocket.Conn, wantLen int) ([]byte, error) {
	var pending []byte
	readMore := func() error {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		pending = append(pending, data...)
		return nil
	}

	for len(pending) < 2 {
		if err := readMore(); err != nil {
			return nil, fmt.Errorf("reading response header: %w", err)
		}
	}
	addonsLen := int(pending[1])
	for len(pending) < 2+addonsLen {
		if err := readMore(); err != nil {
			return nil, fmt.Errorf("reading response addons: %w", err)
		}
	}
	body := pending[2+addonsLen:]
	for len(body) < wantLen {
		if err := readMore(); err != nil {
			return nil, fmt.Errorf("reading response body: %w", err)
		}
		body = pending[2+addonsLen:]
	}
	return body[:wantLen], nil
}
