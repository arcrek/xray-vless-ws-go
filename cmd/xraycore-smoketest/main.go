// Command xraycore-smoketest is a throwaway manual smoke-test binary for
// Phase 2 (embedded xray-core engine). It is NOT wired into the real
// cmd/xrayws binary and should be deleted (or kept under a build tag) once
// Phase 3 needs to route real cloudflared traffic through the engine
// instead.
//
// It proves the full embedding path end-to-end without depending on any
// external xray client: it hand-builds one raw VLESS request (the protocol
// is simple enough to construct by hand — version + UUID + addon-len +
// command + port + address) over a real WebSocket connection to the
// in-process xray-core instance, and confirms the freedom outbound relays
// bytes to a local TCP echo server and back.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
	"github.com/arcrek/xray-vless-ws-go/internal/xraycore"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[smoketest] FAILED: %v", err)
	}
	fmt.Println("[smoketest] PASSED: VLESS+WS handshake round-tripped through the embedded xray-core instance.")
}

func run() error {
	// 1. Local TCP echo server — this is what the freedom outbound will
	// dial once the VLESS request tells it to.
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting echo server: %w", err)
	}
	defer echoLn.Close()
	echoPort := echoLn.Addr().(*net.TCPAddr).Port
	go runEchoServer(echoLn)

	// 2. Build + start the embedded xray-core instance.
	uuidStr := uuid.NewString()
	cfg := &config.Config{
		Ports:    []config.InboundAddr{{ListenIP: "127.0.0.1", Port: 0}},
		XrayUUID: uuidStr,
		WSPath:   "/smoketest",
	}
	// Port 0 lets the OS pick a free port for the echo server, but xray-core
	// itself needs a concrete port to bind — grab one explicitly instead.
	xrayLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserving xray port: %w", err)
	}
	xrayPort := xrayLn.Addr().(*net.TCPAddr).Port
	xrayLn.Close()
	cfg.Ports[0].Port = xrayPort

	cfgBytes, err := xraycore.BuildConfig(cfg)
	if err != nil {
		return fmt.Errorf("BuildConfig: %w", err)
	}
	// This pinned xray-core version added a default security hardening
	// feature (not present when this plan was originally written): a VLESS
	// inbound's freedom outbound blocks private/loopback IP targets by
	// default (anti-SSRF). That's a real, desirable production default —
	// real VLESS clients only ever target public internet addresses — so
	// production BuildConfig deliberately does NOT disable it. This smoke
	// test targets a local echo server on 127.0.0.1, so only *here* we patch
	// in an explicit "allow everything" finalRules override to exercise the
	// data path end-to-end.
	cfgBytes, err = allowPrivateIPsForSmokeTest(cfgBytes)
	if err != nil {
		return fmt.Errorf("patching config for smoke test: %w", err)
	}

	var logLines []string
	engine, err := xraycore.New(cfgBytes, func(line string) {
		logLines = append(logLines, line)
	})
	if err != nil {
		return fmt.Errorf("xraycore.New: %w", err)
	}
	defer func() {
		if cerr := engine.Close(); cerr != nil {
			log.Printf("[smoketest] engine.Close: %v", cerr)
		}
	}()

	// Give xray-core's inbound listener a moment to actually bind.
	time.Sleep(300 * time.Millisecond)

	// 3. Dial the VLESS+WS inbound as a raw WebSocket client.
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/smoketest", xrayPort)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dialing WS %s: %w (log lines: %v)", wsURL, err, logLines)
	}
	defer conn.Close()

	// 4. Hand-build a VLESS request targeting 127.0.0.1:<echoPort>.
	req, err := buildVLESSRequest(uuidStr, "127.0.0.1", echoPort)
	if err != nil {
		return fmt.Errorf("building VLESS request: %w", err)
	}
	payload := []byte("PING-FROM-SMOKETEST")
	if err := conn.WriteMessage(websocket.BinaryMessage, append(req, payload...)); err != nil {
		return fmt.Errorf("writing VLESS request: %w", err)
	}

	// 5. Read the VLESS response header + echoed payload.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading VLESS response: %w (log lines: %v)", err, logLines)
	}

	body, err := stripVLESSResponseHeader(data)
	if err != nil {
		return fmt.Errorf("parsing VLESS response: %w (raw=%x)", err, data)
	}

	// The echo server may split the reply across frames/reads; accumulate
	// until we see the full payload or time out.
	for !bytes.Contains(body, payload) {
		_, more, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("reading follow-up frame: %w (got so far: %q)", err, body)
		}
		body = append(body, more...)
	}

	if !bytes.Contains(body, payload) {
		return fmt.Errorf("echoed payload mismatch: got %q, want to contain %q", body, payload)
	}

	// 6. Close and confirm the port is released immediately.
	if err := engine.Close(); err != nil {
		return fmt.Errorf("engine.Close: %w", err)
	}
	ln2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", xrayPort))
	if err != nil {
		return fmt.Errorf("port %d not released after Close: %w", xrayPort, err)
	}
	ln2.Close()

	return nil
}

// allowPrivateIPsForSmokeTest injects a "finalRules": [{"action": "allow"}]
// override into the config's single freedom outbound, so this smoke test
// can target a loopback echo server despite xray-core's default
// private-IP block on VLESS-sourced traffic (see call site comment).
func allowPrivateIPsForSmokeTest(cfgBytes []byte) ([]byte, error) {
	var decoded map[string]any
	if err := json.Unmarshal(cfgBytes, &decoded); err != nil {
		return nil, err
	}
	outbounds, ok := decoded["outbounds"].([]any)
	if !ok || len(outbounds) == 0 {
		return nil, fmt.Errorf("no outbounds in config")
	}
	outbound := outbounds[0].(map[string]any)
	settings := outbound["settings"].(map[string]any)
	settings["finalRules"] = []map[string]any{{"action": "allow"}}
	return json.Marshal(decoded)
}

func runEchoServer(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			io.Copy(c, c)
		}(c)
	}
}

// buildVLESSRequest hand-builds a minimal VLESS request header:
// version(1) + uuid(16) + addon-len(1)=0 + command(1)=TCP(1) + port(2 BE) +
// addrType(1)=IPv4(1) + addr(4).
func buildVLESSRequest(uuidStr, ip string, port int) ([]byte, error) {
	id, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, err
	}
	parsedIP := net.ParseIP(ip).To4()
	if parsedIP == nil {
		return nil, fmt.Errorf("only IPv4 target supported by this smoke test, got %q", ip)
	}

	buf := new(bytes.Buffer)
	buf.WriteByte(0) // version
	idBytes := id
	buf.Write(idBytes[:])
	buf.WriteByte(0) // addon length
	buf.WriteByte(1) // command: TCP
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	buf.Write(portBytes)
	buf.WriteByte(1) // address type: IPv4
	buf.Write(parsedIP)
	return buf.Bytes(), nil
}

// stripVLESSResponseHeader strips the VLESS response header:
// version(1) + addon-len(1) + addons(addon-len), returning the remaining
// payload bytes.
func stripVLESSResponseHeader(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("response too short: %d bytes", len(data))
	}
	addonLen := int(data[1])
	if len(data) < 2+addonLen {
		return nil, fmt.Errorf("response shorter than declared addon length")
	}
	return data[2+addonLen:], nil
}
