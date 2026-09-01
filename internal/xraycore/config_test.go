package xraycore

import (
	"encoding/json"
	"testing"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	ports, err := config.ParsePorts("0.0.0.0:8888,0.0.0.0:8889")
	if err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		Ports:    ports,
		XrayUUID: "test-uuid",
		WSPath:   "/tiktok4g",
	}
}

func TestBuildConfigShape(t *testing.T) {
	cfg := testConfig(t)
	raw, err := BuildConfig(cfg)
	if err != nil {
		t.Fatalf("BuildConfig: unexpected error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("BuildConfig produced invalid JSON: %v", err)
	}

	inbounds, ok := decoded["inbounds"].([]any)
	if !ok || len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds, got %#v", decoded["inbounds"])
	}

	first := inbounds[0].(map[string]any)
	if first["protocol"] != "vless" {
		t.Errorf("inbound[0].protocol = %v, want vless", first["protocol"])
	}
	if first["port"] != float64(8888) {
		t.Errorf("inbound[0].port = %v, want 8888", first["port"])
	}

	settings := first["settings"].(map[string]any)
	if settings["decryption"] != "none" {
		t.Errorf("settings.decryption = %v, want none", settings["decryption"])
	}
	clients := settings["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	client := clients[0].(map[string]any)
	if client["id"] != "test-uuid" {
		t.Errorf("client.id = %v, want test-uuid", client["id"])
	}
	if client["email"] != statsEmail {
		t.Errorf("client.email = %v, want %v", client["email"], statsEmail)
	}
	if client["level"] != float64(0) {
		t.Errorf("client.level = %v, want 0", client["level"])
	}

	stream := first["streamSettings"].(map[string]any)
	if stream["network"] != "ws" {
		t.Errorf("streamSettings.network = %v, want ws", stream["network"])
	}
	ws := stream["wsSettings"].(map[string]any)
	if ws["path"] != "/tiktok4g" {
		t.Errorf("wsSettings.path = %v, want /tiktok4g", ws["path"])
	}
	if hb, ok := ws["heartbeatPeriod"]; !ok || hb != float64(15) {
		t.Errorf("wsSettings.heartbeatPeriod = %v, want 15", hb)
	}

	outbounds, ok := decoded["outbounds"].([]any)
	if !ok || len(outbounds) != 1 {
		t.Fatalf("expected exactly 1 outbound (no WARP), got %#v", decoded["outbounds"])
	}
	outbound := outbounds[0].(map[string]any)
	if outbound["protocol"] != "freedom" {
		t.Errorf("outbound.protocol = %v, want freedom (WARP dropped per decision log #5)", outbound["protocol"])
	}

	// Exact-value assertions on the new policy/stats keys (plan Phase 1,
	// Red Team finding #5) — not just "keys present", to actually guard
	// against the pinned xray-core API drifting the JSON shape.
	policy, ok := decoded["policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level policy block, got %#v", decoded["policy"])
	}
	levels, ok := policy["levels"].(map[string]any)
	if !ok {
		t.Fatalf("expected policy.levels, got %#v", policy["levels"])
	}
	level0, ok := levels["0"].(map[string]any)
	if !ok {
		t.Fatalf("expected policy.levels[\"0\"], got %#v", levels["0"])
	}
	if level0["statsUserUplink"] != true {
		t.Errorf("policy.levels[0].statsUserUplink = %v, want true", level0["statsUserUplink"])
	}
	if level0["statsUserDownlink"] != true {
		t.Errorf("policy.levels[0].statsUserDownlink = %v, want true", level0["statsUserDownlink"])
	}
	if level0["connIdle"] != float64(300) {
		t.Errorf("policy.levels[0].connIdle = %v, want 300", level0["connIdle"])
	}
	if level0["downlinkOnly"] != float64(5) {
		t.Errorf("policy.levels[0].downlinkOnly = %v, want 5", level0["downlinkOnly"])
	}
	if level0["uplinkOnly"] != float64(2) {
		t.Errorf("policy.levels[0].uplinkOnly = %v, want 2", level0["uplinkOnly"])
	}

	stats, ok := decoded["stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level stats block, got %#v", decoded["stats"])
	}
	if len(stats) != 0 {
		t.Errorf("stats block = %#v, want empty object", stats)
	}
}

func TestBuildConfigRejectsEmptyPorts(t *testing.T) {
	cfg := &config.Config{XrayUUID: "x", WSPath: "/p"}
	if _, err := BuildConfig(cfg); err == nil {
		t.Fatal("BuildConfig: expected error for config with no ports, got nil")
	}
}
