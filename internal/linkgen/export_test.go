package linkgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "frp_info.config")
	jsonPath := filepath.Join(dir, "frp_info.json")

	links := []string{"vless://a", "vless://b"}
	meta := ExportMeta{IP: "1.2.3.4", WSHost: "host.trycloudflare.com", WSPath: "/p", Transport: "websocket", StartTime: 1000}

	if err := Export(links, meta, configPath, jsonPath); err != nil {
		t.Fatalf("Export: unexpected error: %v", err)
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	want := "vless://a\nvless://b"
	if string(configBytes) != want {
		t.Errorf("config content = %q, want %q (no trailing newline, matching main.py)", string(configBytes), want)
	}

	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("reading json file: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json file is not valid JSON: %v", err)
	}
	if decoded["ip"] != "1.2.3.4" {
		t.Errorf("ip = %v, want 1.2.3.4", decoded["ip"])
	}
	if decoded["wshost"] != "host.trycloudflare.com" {
		t.Errorf("wshost = %v, want host.trycloudflare.com", decoded["wshost"])
	}
	payloads, ok := decoded["payloads"].([]any)
	if !ok || len(payloads) != 2 {
		t.Errorf("payloads = %v, want 2-element array", decoded["payloads"])
	}
}

func TestExportConfigWriteFailureDoesNotBlockJSONWrite(t *testing.T) {
	dir := t.TempDir()
	// A directory path where a file is expected forces the config write to
	// fail, independent of the JSON write.
	badConfigPath := dir // writing to a directory path fails
	jsonPath := filepath.Join(dir, "frp_info.json")

	err := Export([]string{"vless://a"}, ExportMeta{IP: "1.2.3.4"}, badConfigPath, jsonPath)
	if err == nil {
		t.Fatal("expected an error from the failed config write, got nil")
	}

	if _, statErr := os.Stat(jsonPath); statErr != nil {
		t.Errorf("json file should still have been written despite config write failure: %v", statErr)
	}
}
