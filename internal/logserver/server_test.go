package logserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
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
	cfgPath := filepath.Join(tmpDir, "vless_info.config")
	jsonPath := filepath.Join(tmpDir, "vless_info.json")

	s := New(addr, "", 500)
	s.Domain = "example.com"
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

	// 2. Write mock vless_info.config and vless_info.json
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
		SubPath      string   `json:"sub_path"`
		WorkerSubURL string   `json:"worker_sub_url"`
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
	if infoResp2.SubPath != "/sub" {
		t.Errorf("expected sub_path /sub, got %s", infoResp2.SubPath)
	}
	if infoResp2.WorkerSubURL != "https://vless.example.com/sub" {
		t.Errorf("expected worker_sub_url https://vless.example.com/sub, got %s", infoResp2.WorkerSubURL)
	}
}

func TestServerSubscriptionEndpoint(t *testing.T) {
	addr := freeLocalAddr(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "vless_info.config")

	s := New(addr, "secret-password", 500)
	s.ConfigPath = cfgPath

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Start(ctx)
	waitForServer(t, addr)

	// 1. Before file exists -> 503 Service Unavailable (unauthenticated endpoint)
	resp503, err := http.Get(fmt.Sprintf("http://%s/sub", addr))
	if err != nil {
		t.Fatalf("GET /sub: %v", err)
	}
	if resp503.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 before config exists, got %d", resp503.StatusCode)
	}
	resp503.Body.Close()

	// 2. Write vless_info.config
	link1 := "vless://uuid-1@tunnel.example.com:443?type=ws&security=tls#TLS"
	link2 := "vless://uuid-2@tunnel.example.com:80?type=ws&security=#NO%20TLS"
	configContent := link1 + "\n" + link2
	if err := os.WriteFile(cfgPath, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. GET /sub (base64, check headers)
	respSub, err := http.Get(fmt.Sprintf("http://%s/sub", addr))
	if err != nil {
		t.Fatalf("GET /sub: %v", err)
	}
	defer respSub.Body.Close()

	if respSub.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", respSub.StatusCode)
	}
	if ct := respSub.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %s", ct)
	}
	if interval := respSub.Header.Get("Profile-Update-Interval"); interval != "24" {
		t.Errorf("expected Profile-Update-Interval 24, got %s", interval)
	}
	if cors := respSub.Header.Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("expected Access-Control-Allow-Origin *, got %s", cors)
	}
	if userInfo := respSub.Header.Get("Subscription-Userinfo"); userInfo == "" {
		t.Errorf("expected non-empty Subscription-Userinfo header")
	}

	b64Body, err := io.ReadAll(respSub.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(b64Body))
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}
	if string(decoded) != configContent {
		t.Errorf("decoded base64 = %q, want %q", string(decoded), configContent)
	}

	// 4. GET /subscription alias
	respAlias, err := http.Get(fmt.Sprintf("http://%s/subscription", addr))
	if err != nil {
		t.Fatalf("GET /subscription: %v", err)
	}
	aliasBody, _ := io.ReadAll(respAlias.Body)
	respAlias.Body.Close()
	if string(aliasBody) != string(b64Body) {
		t.Errorf("alias /subscription did not match /sub")
	}

	// 5. GET /sub?raw=1
	respRaw, err := http.Get(fmt.Sprintf("http://%s/sub?raw=1", addr))
	if err != nil {
		t.Fatalf("GET /sub?raw=1: %v", err)
	}
	rawBody, _ := io.ReadAll(respRaw.Body)
	respRaw.Body.Close()
	if string(rawBody) != configContent {
		t.Errorf("raw response = %q, want %q", string(rawBody), configContent)
	}

	// 6. Test fallback to frp_info.config when vless_info.config is absent and default path is used
	addrFallback := freeLocalAddr(t)
	sFallback := New(addrFallback, "", 500)
	// Run in a temp dir where only frp_info.config exists
	fallbackDir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(fallbackDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	_ = os.WriteFile("frp_info.config", []byte("vless://legacy-link"), 0o644)
	_ = os.WriteFile("frp_info.json", []byte(`{"ip":"5.6.7.8","wshost":"legacy.com"}`), 0o644)

	ctxFallback, cancelFallback := context.WithCancel(context.Background())
	defer cancelFallback()
	go sFallback.Start(ctxFallback)
	waitForServer(t, addrFallback)

	respFB, err := http.Get(fmt.Sprintf("http://%s/sub?raw=true", addrFallback))
	if err != nil {
		t.Fatalf("GET fallback /sub: %v", err)
	}
	fbBody, _ := io.ReadAll(respFB.Body)
	respFB.Body.Close()
	if string(fbBody) != "vless://legacy-link" {
		t.Errorf("fallback body = %q, want %q", string(fbBody), "vless://legacy-link")
	}

	// Also check fallback in /api/vless-info
	respVlessFB, err := http.Get(fmt.Sprintf("http://%s/api/vless-info", addrFallback))
	if err != nil {
		t.Fatalf("GET /api/vless-info fallback: %v", err)
	}
	var vlessFB struct {
		Ready     bool   `json:"ready"`
		RawConfig string `json:"raw_config"`
		WSHost    string `json:"wshost"`
	}
	json.NewDecoder(respVlessFB.Body).Decode(&vlessFB)
	respVlessFB.Body.Close()
	if !vlessFB.Ready || vlessFB.RawConfig != "vless://legacy-link" || vlessFB.WSHost != "legacy.com" {
		t.Errorf("vlessFB fallback mismatch: %+v", vlessFB)
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
