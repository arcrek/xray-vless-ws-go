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

	stream := first["streamSettings"].(map[string]any)
	if stream["network"] != "ws" {
		t.Errorf("streamSettings.network = %v, want ws", stream["network"])
	}
	ws := stream["wsSettings"].(map[string]any)
	if ws["path"] != "/tiktok4g" {
		t.Errorf("wsSettings.path = %v, want /tiktok4g", ws["path"])
	}

	outbounds, ok := decoded["outbounds"].([]any)
	if !ok || len(outbounds) != 1 {
		t.Fatalf("expected exactly 1 outbound (no WARP), got %#v", decoded["outbounds"])
	}
	outbound := outbounds[0].(map[string]any)
	if outbound["protocol"] != "freedom" {
		t.Errorf("outbound.protocol = %v, want freedom (WARP dropped per decision log #5)", outbound["protocol"])
	}
}

func TestBuildConfigRejectsEmptyPorts(t *testing.T) {
	cfg := &config.Config{XrayUUID: "x", WSPath: "/p"}
	if _, err := BuildConfig(cfg); err == nil {
		t.Fatal("BuildConfig: expected error for config with no ports, got nil")
	}
}
