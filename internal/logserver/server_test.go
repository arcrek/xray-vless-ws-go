package logserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestServerServesIndexAndLogs(t *testing.T) {
	addr := freeLocalAddr(t)
	s := New(addr, "", 500)
	s.Push("XRAY", "hello xray")
	s.Push("CLOUDFLARE", "hello cloudflare")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	resp, err := http.Get(fmt.Sprintf("http://%s/", addr))
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want 200", resp.StatusCode)
	}

	resp2, err := http.Get(fmt.Sprintf("http://%s/logs?last_id=0", addr))
	if err != nil {
		t.Fatalf("GET /logs: %v", err)
	}
	defer resp2.Body.Close()
	var body struct {
		NewLogs []LogLine `json:"new_logs"`
		LastID  int       `json:"last_id"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatalf("decoding /logs response: %v", err)
	}
	if len(body.NewLogs) != 2 || body.LastID != 2 {
		t.Errorf("got %+v, want 2 new_logs and last_id=2", body)
	}
}

func TestServerBasicAuth(t *testing.T) {
	addr := freeLocalAddr(t)
	s := New(addr, "secret", 500)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	resp, err := http.Get(fmt.Sprintf("http://%s/", addr))
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET / status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/", addr), nil)
	req.SetBasicAuth("anyuser", "secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authenticated GET /: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("authenticated GET / status = %d, want 200", resp2.StatusCode)
	}
}

func TestServerServesStats(t *testing.T) {
	addr := freeLocalAddr(t)
	s := New(addr, "secret", 500)
	s.Status.SetXrayUp(true)
	s.Status.SetTunnelStatus(true, 1)
	s.Status.SetHostname("test.trycloudflare.com")
	s.Status.RecordTraffic(100, 200)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	// Same auth as /logs — unauthenticated request is rejected.
	resp, err := http.Get(fmt.Sprintf("http://%s/stats", addr))
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /stats status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/stats", addr), nil)
	req.SetBasicAuth("anyuser", "secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authenticated GET /stats: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET /stats status = %d, want 200", resp2.StatusCode)
	}

	var snap StatsSnapshot
	if err := json.NewDecoder(resp2.Body).Decode(&snap); err != nil {
		t.Fatalf("decoding /stats response: %v", err)
	}
	if !snap.XrayUp || !snap.TunnelReady || snap.ReadyConnections != 1 {
		t.Errorf("status fields = %+v, want XrayUp/TunnelReady true, ReadyConnections=1", snap)
	}
	if snap.Hostname != "test.trycloudflare.com" {
		t.Errorf("hostname = %q, want test.trycloudflare.com", snap.Hostname)
	}
	if snap.UplinkTotal != 100 || snap.DownlinkTotal != 200 {
		t.Errorf("totals = (%d, %d), want (100, 200)", snap.UplinkTotal, snap.DownlinkTotal)
	}
	if snap.History == nil {
		t.Error("history decoded as nil, want a non-nil (possibly empty) slice — [] must not serialize as null")
	}
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server on %s did not start in time", addr)
}
