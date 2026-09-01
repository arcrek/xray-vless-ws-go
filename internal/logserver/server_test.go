package logserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
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

func TestServerCookieLoginAndLogout(t *testing.T) {
	addr := freeLocalAddr(t)
	s := New(addr, "secret123", 500)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Unauthenticated request to /stats should return 401
	resp, err := client.Get(fmt.Sprintf("http://%s/stats", addr))
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET /stats = %d, want 401", resp.StatusCode)
	}

	// 2. Auth status check -> auth_required: true, authenticated: false
	respStatus, err := client.Get(fmt.Sprintf("http://%s/api/auth-status", addr))
	if err != nil {
		t.Fatalf("GET /api/auth-status: %v", err)
	}
	var statusBody struct {
		AuthRequired  bool `json:"auth_required"`
		Authenticated bool `json:"authenticated"`
	}
	json.NewDecoder(respStatus.Body).Decode(&statusBody)
	respStatus.Body.Close()
	if !statusBody.AuthRequired || statusBody.Authenticated {
		t.Errorf("got auth status %+v, want required=true, auth=false", statusBody)
	}

	// 3. Login with wrong password
	badLoginPayload, _ := json.Marshal(map[string]string{"password": "wrong"})
	respBadLogin, err := client.Post(fmt.Sprintf("http://%s/api/login", addr), "application/json", bytes.NewReader(badLoginPayload))
	if err != nil {
		t.Fatalf("POST /api/login (bad): %v", err)
	}
	respBadLogin.Body.Close()
	if respBadLogin.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/login bad password status = %d, want 401", respBadLogin.StatusCode)
	}

	// 4. Login with correct password
	goodLoginPayload, _ := json.Marshal(map[string]string{"password": "secret123"})
	respGoodLogin, err := client.Post(fmt.Sprintf("http://%s/api/login", addr), "application/json", bytes.NewReader(goodLoginPayload))
	if err != nil {
		t.Fatalf("POST /api/login (good): %v", err)
	}
	var loginResp struct {
		OK    bool   `json:"ok"`
		Token string `json:"token"`
	}
	json.NewDecoder(respGoodLogin.Body).Decode(&loginResp)
	respGoodLogin.Body.Close()
	if !loginResp.OK || loginResp.Token == "" {
		t.Fatalf("login response failed: %+v", loginResp)
	}

	// 5. Now authenticated request to /stats should succeed
	respAuthStats, err := client.Get(fmt.Sprintf("http://%s/stats", addr))
	if err != nil {
		t.Fatalf("GET /stats authenticated: %v", err)
	}
	respAuthStats.Body.Close()
	if respAuthStats.StatusCode != http.StatusOK {
		t.Errorf("authenticated GET /stats status = %d, want 200", respAuthStats.StatusCode)
	}

	// 6. Logout
	respLogout, err := client.Post(fmt.Sprintf("http://%s/api/logout", addr), "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/logout: %v", err)
	}
	respLogout.Body.Close()
	if respLogout.StatusCode != http.StatusOK {
		t.Errorf("POST /api/logout status = %d, want 200", respLogout.StatusCode)
	}

	// 7. Request to /stats after logout should be 401
	respAfterLogout, err := client.Get(fmt.Sprintf("http://%s/stats", addr))
	if err != nil {
		t.Fatalf("GET /stats after logout: %v", err)
	}
	respAfterLogout.Body.Close()
	if respAfterLogout.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /stats after logout status = %d, want 401", respAfterLogout.StatusCode)
	}
}

func TestServerBasicAuthBackwardCompatibility(t *testing.T) {
	addr := freeLocalAddr(t)
	s := New(addr, "secret", 500)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/stats", addr), nil)
	req.SetBasicAuth("anyuser", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authenticated GET /stats: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("authenticated GET /stats with basic auth status = %d, want 200", resp.StatusCode)
	}
}

func TestServerVlessInfoEndpoint(t *testing.T) {
	addr := freeLocalAddr(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "frp_info.config")
	jsonPath := filepath.Join(tmpDir, "frp_info.json")

	s := New(addr, "", 500)
	s.ConfigPath = cfgPath
	s.JSONPath = jsonPath

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	// 1. Before file exists
	resp, err := http.Get(fmt.Sprintf("http://%s/api/vless-info", addr))
	if err != nil {
		t.Fatalf("GET /api/vless-info: %v", err)
	}
	var infoResp1 struct {
		Ready bool `json:"ready"`
	}
	json.NewDecoder(resp.Body).Decode(&infoResp1)
	resp.Body.Close()
	if infoResp1.Ready {
		t.Errorf("expected ready=false before file written")
	}

	// 2. Write mock frp_info.config and frp_info.json
	link1 := "vless://uuid-1@tunnel.example.com:443?type=ws&security=tls#Node-1"
	link2 := "vless://uuid-2@tunnel.example.com:443?type=ws&security=tls#Node-2"
	_ = os.WriteFile(cfgPath, []byte(link1+"\n"+link2), 0o644)
	metaJSON := `{"ip":"1.2.3.4","wshost":"tunnel.example.com","wspath":"/vless","transport":"websocket"}`
	_ = os.WriteFile(jsonPath, []byte(metaJSON), 0o644)

	resp2, err := http.Get(fmt.Sprintf("http://%s/api/vless-info", addr))
	if err != nil {
		t.Fatalf("GET /api/vless-info after file written: %v", err)
	}
	var infoResp2 struct {
		Ready        bool     `json:"ready"`
		Links        []string `json:"links"`
		RawConfig    string   `json:"raw_config"`
		Base64Config string   `json:"base64_config"`
		IP           string   `json:"ip"`
		WSHost       string   `json:"wshost"`
		WSPath       string   `json:"wspath"`
	}
	json.NewDecoder(resp2.Body).Decode(&infoResp2)
	resp2.Body.Close()

	if !infoResp2.Ready {
		t.Errorf("expected ready=true")
	}
	if len(infoResp2.Links) != 2 {
		t.Errorf("expected 2 links, got %d", len(infoResp2.Links))
	}
	if infoResp2.Base64Config == "" {
		t.Errorf("expected non-empty base64 config")
	}
	if infoResp2.WSHost != "tunnel.example.com" {
		t.Errorf("expected wshost tunnel.example.com, got %s", infoResp2.WSHost)
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
