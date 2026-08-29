package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePorts(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []InboundAddr
		wantErr bool
	}{
		{"bare port", "8888", []InboundAddr{{"0.0.0.0", 8888}}, false},
		{"ip:port", "127.0.0.1:8888", []InboundAddr{{"127.0.0.1", 8888}}, false},
		{"multi-port csv", "0.0.0.0:443,0.0.0.0:80", []InboundAddr{{"0.0.0.0", 443}, {"0.0.0.0", 80}}, false},
		{"mixed csv with spaces", "8888, 127.0.0.1:9999", []InboundAddr{{"0.0.0.0", 8888}, {"127.0.0.1", 9999}}, false},
		{"malformed", "not-a-port", nil, true},
		{"empty", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePorts(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePorts(%q): expected error, got none", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePorts(%q): unexpected error: %v", tc.raw, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParsePorts(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("ParsePorts(%q)[%d] = %+v, want %+v", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTunnelTarget(t *testing.T) {
	ip, port := TunnelTarget([]InboundAddr{{"0.0.0.0", 8888}})
	if ip != "127.0.0.1" || port != 8888 {
		t.Errorf("TunnelTarget wildcard: got %s:%d, want 127.0.0.1:8888", ip, port)
	}
	ip, port = TunnelTarget([]InboundAddr{{"127.0.0.1", 8888}})
	if ip != "127.0.0.1" || port != 8888 {
		t.Errorf("TunnelTarget non-wildcard: got %s:%d, want 127.0.0.1:8888", ip, port)
	}
}

func TestParseSNIList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []SNIEntry
		wantErr bool
	}{
		{
			"with remark",
			"a.com#Remark A,b.com#Remark B",
			[]SNIEntry{{"a.com", "Remark A"}, {"b.com", "Remark B"}},
			false,
		},
		{
			"bare no remark",
			"a.com,b.com",
			[]SNIEntry{{"a.com", "Tunnel 1"}, {"b.com", "Tunnel 2"}},
			false,
		},
		{
			"empty remark after hash",
			"a.com#",
			[]SNIEntry{{"a.com", "Tunnel 1"}},
			false,
		},
		{"empty", "", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSNIList(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSNIList(%q): expected error, got none", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSNIList(%q): unexpected error: %v", tc.raw, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseSNIList(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("ParseSNIList(%q)[%d] = %+v, want %+v", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// envKeys lists every env var config.Load reads. godotenv.Load does not
// override a var already set in the process environment (by design) —
// since Go tests share one process, an earlier subtest's os.Setenv leaks
// into later ones unless cleared first. clearEnvKeys
// simulates the fresh-process env every real run actually has.
var envKeys = []string{
	"PORT", "XRAY_UUID", "FAKE_SNI", "WS_PATH", "WS_HOST", "TRANSPORT", "WEBHOOK_URL", "TUNNEL_TOKEN", "DEBUG_MODE", "LOG_PASSWORD",
	"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID", "DOMAIN", "WORKER_PASSWORD",
}

func clearEnvKeys(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			}
		})
	}
}

func TestLoadCreatesDefaultEnv(t *testing.T) {
	clearEnvKeys(t)
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Errorf(".env was not created: %v", err)
	}
	if len(cfg.Ports) != 1 || cfg.Ports[0].Port != 8888 {
		t.Errorf("unexpected default Ports: %+v", cfg.Ports)
	}
	if cfg.WSPath != "/tiktok4g" {
		t.Errorf("unexpected default WSPath: %q", cfg.WSPath)
	}
	if cfg.TunnelToken != "" {
		t.Errorf("expected empty default TunnelToken, got %q", cfg.TunnelToken)
	}
}

func TestLoadMalformedPortDoesNotPanic(t *testing.T) {
	clearEnvKeys(t)
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	if err := os.WriteFile(".env", []byte("PORT=garbage\nXRAY_UUID=x\nFAKE_SNI=a.com\nWS_PATH=/p\nWS_HOST=h\nTRANSPORT=websocket\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load(): expected error for malformed PORT, got nil")
	}
}

func TestWSPathNormalization(t *testing.T) {
	clearEnvKeys(t)
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	if err := os.WriteFile(".env", []byte("PORT=8888\nXRAY_UUID=x\nFAKE_SNI=a.com\nWS_PATH=noSlash\nWS_HOST=h\nTRANSPORT=websocket\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.WSPath != "/noSlash" {
		t.Errorf("WSPath = %q, want /noSlash", cfg.WSPath)
	}
}

func TestCloudflareAutoDeployFieldsDefaultOff(t *testing.T) {
	clearEnvKeys(t)
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.CloudflareAPIToken != "" {
		t.Errorf("expected empty default CloudflareAPIToken, got %q", cfg.CloudflareAPIToken)
	}
	if cfg.Domain != "" {
		t.Errorf("expected empty default Domain, got %q", cfg.Domain)
	}
	if cfg.CloudflareAccountID != "" {
		t.Errorf("expected empty default CloudflareAccountID, got %q", cfg.CloudflareAccountID)
	}
	// WorkerPassword is generated even when the feature is off — same
	// "always present, harmless if unused" idiom as XRAY_UUID.
	if cfg.WorkerPassword == "" {
		t.Error("expected a generated default WorkerPassword, got empty string")
	}
}

func TestCloudflareAutoDeployFieldsFromEnv(t *testing.T) {
	clearEnvKeys(t)
	dir := t.TempDir()
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	envContent := "PORT=8888\nXRAY_UUID=x\nFAKE_SNI=a.com\nWS_PATH=/p\nWS_HOST=h\nTRANSPORT=websocket\n" +
		"CLOUDFLARE_API_TOKEN= tok123 \nCLOUDFLARE_ACCOUNT_ID= acc1 \nDOMAIN= example.com \nWORKER_PASSWORD=secret1\n"
	if err := os.WriteFile(".env", []byte(envContent), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): unexpected error: %v", err)
	}
	if cfg.CloudflareAPIToken != "tok123" {
		t.Errorf("CloudflareAPIToken = %q, want tok123 (trimmed)", cfg.CloudflareAPIToken)
	}
	if cfg.CloudflareAccountID != "acc1" {
		t.Errorf("CloudflareAccountID = %q, want acc1 (trimmed)", cfg.CloudflareAccountID)
	}
	if cfg.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com (trimmed)", cfg.Domain)
	}
	if cfg.WorkerPassword != "secret1" {
		t.Errorf("WorkerPassword = %q, want secret1", cfg.WorkerPassword)
	}
}
