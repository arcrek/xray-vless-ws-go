package ci

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportSecretEnvWritesFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	os.Setenv("ENV_CONFIG", "  PORT=1234\nXRAY_UUID=abc\n  ")
	defer os.Unsetenv("ENV_CONFIG")

	if err := ExportSecretEnv(envPath); err != nil {
		t.Fatalf("ExportSecretEnv: %v", err)
	}

	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "PORT=1234\nXRAY_UUID=abc"
	if string(content) != want {
		t.Errorf("content = %q, want %q (trimmed)", content, want)
	}
}

func TestExportSecretEnvSkipsWhenUnset(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.Unsetenv("ENV_CONFIG")

	if err := ExportSecretEnv(envPath); err != nil {
		t.Fatalf("ExportSecretEnv: %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Errorf(".env should not have been created when ENV_CONFIG is unset")
	}
}
