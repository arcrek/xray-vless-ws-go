package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteNamedTunnelConfigHasNoTunnelField is a regression guard: putting
// the connector token in config.yml's `tunnel:` field (instead of passing
// it via Launch's --token flag) makes cloudflared try to resolve that value
// as a tunnel UUID/name needing a local cert.pem, and fail with
// "error parsing tunnel ID: Error locating origin cert" — a live bug here
// until the named-tunnel path was exercised end-to-end with a real token.
func TestWriteNamedTunnelConfigHasNoTunnelField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	if err := WriteNamedTunnelConfig(path, "tunnel.example.com", "127.0.0.1", 8888); err != nil {
		t.Fatalf("WriteNamedTunnelConfig: unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config.yml: %v", err)
	}
	s := string(content)

	if strings.Contains(s, "tunnel:") {
		t.Errorf("config.yml must not contain a `tunnel:` field (that's for a UUID/name + cert.pem, not the token):\n%s", s)
	}
	if !strings.Contains(s, "hostname: tunnel.example.com") {
		t.Errorf("config.yml missing expected ingress hostname:\n%s", s)
	}
	if !strings.Contains(s, "service: http://127.0.0.1:8888") {
		t.Errorf("config.yml missing expected ingress service:\n%s", s)
	}
}

func TestLaunchNamedTunnelPassesTokenAsFlagNotInConfig(t *testing.T) {
	binPath := buildFakeCloudflared(t)
	workDir := t.TempDir()

	cfg := testSupervisorConfig(t)
	cfg.TunnelToken = "fake-token-for-test"
	cfg.WSHost = "tunnel.example.com"

	h, err := Launch(cfg, binPath, workDir)
	if err != nil {
		t.Fatalf("Launch: unexpected error: %v", err)
	}
	t.Cleanup(func() { h.Kill(); h.Wait() })

	args := h.Cmd.Args
	found := false
	for i, a := range args {
		if a == "--token" {
			found = true
			if i+1 >= len(args) || args[i+1] != cfg.TunnelToken {
				t.Errorf("--token flag not followed by the configured token, args = %v", args)
			}
		}
	}
	if !found {
		t.Errorf("expected --token in cloudflared args, got %v", args)
	}

	content, err := os.ReadFile(filepath.Join(workDir, "config.yml"))
	if err != nil {
		t.Fatalf("reading config.yml: %v", err)
	}
	if strings.Contains(string(content), cfg.TunnelToken) {
		t.Errorf("token must not be written into config.yml, got:\n%s", content)
	}
}
