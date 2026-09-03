package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplaceSuccess(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "xrayws")
	if err := os.WriteFile(execPath, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	tmpPath := filepath.Join(dir, "xrayws-update-tmp")
	if err := os.WriteFile(tmpPath, []byte("new binary"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := atomicReplace(execPath, tmpPath); err != nil {
		t.Fatalf("atomicReplace: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("execPath content = %q, want %q", got, "new binary")
	}

	info, err := os.Stat(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Errorf("execPath not executable: mode=%v", info.Mode())
	}

	prev, err := os.ReadFile(execPath + ".prev")
	if err != nil {
		t.Fatalf("reading .prev: %v", err)
	}
	if string(prev) != "old binary" {
		t.Errorf(".prev content = %q, want %q", prev, "old binary")
	}

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tmpPath should be consumed by rename, stat err = %v", err)
	}
}

func TestAtomicReplaceReplacesStalePrev(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "xrayws")
	os.WriteFile(execPath, []byte("v2"), 0755)
	os.WriteFile(execPath+".prev", []byte("v1-stale"), 0755)
	tmpPath := filepath.Join(dir, "xrayws-update-tmp")
	os.WriteFile(tmpPath, []byte("v3"), 0644)

	if err := atomicReplace(execPath, tmpPath); err != nil {
		t.Fatalf("atomicReplace: %v", err)
	}

	prev, _ := os.ReadFile(execPath + ".prev")
	if string(prev) != "v2" {
		t.Errorf(".prev = %q, want %q (the old current binary, not the stale one)", prev, "v2")
	}
}

func TestAtomicReplaceLeavesExecPathUntouchedOnChmodFailure(t *testing.T) {
	dir := t.TempDir()
	execPath := filepath.Join(dir, "xrayws")
	os.WriteFile(execPath, []byte("old binary"), 0755)
	// tmpPath deliberately doesn't exist -> os.Chmod fails before any rename.
	tmpPath := filepath.Join(dir, "does-not-exist")

	if err := atomicReplace(execPath, tmpPath); err == nil {
		t.Fatal("expected error for missing tmpPath, got nil")
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old binary" {
		t.Errorf("execPath was modified despite failure: %q", got)
	}
}
