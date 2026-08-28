package tunnel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestEnsureBinaryReturnsAbsolutePath guards a real bug found by an
// independent test pass: EnsureBinary(ctx, ".") used to return
// filepath.Join(".", "cloudflared"), which Go's filepath.Join collapses to
// the bare string "cloudflared" (no path separator). os/exec only resolves
// a binary relative to the caller's working directory when the string
// contains a separator; a bare name is looked up on $PATH instead — so the
// binary EnsureBinary had just downloaded into the working directory could
// never actually be found by Launch()'s exec.Command call. This silently
// broke tunnel launch in the default (fresh install, no system-wide
// cloudflared) case.
func TestEnsureBinaryReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()

	asset, err := currentAsset()
	if err != nil {
		t.Skipf("currentAsset() unsupported on this OS/arch: %v", err)
	}

	// Pre-place a fake binary so EnsureBinary takes the "already exists"
	// fast path — no network needed for this test.
	fakeBin := filepath.Join(dir, asset.localName)
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Exercise it exactly like main.go does: a relative "." dir.
	oldWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	got, err := EnsureBinary(context.Background(), ".")
	if err != nil {
		t.Fatalf("EnsureBinary: %v", err)
	}

	if !filepath.IsAbs(got) {
		t.Fatalf("EnsureBinary returned a non-absolute path %q — exec.Command would resolve this via $PATH lookup instead of the downloaded binary", got)
	}

	// The real-world assertion: exec.Command must find and run it WITHOUT
	// any help from $PATH (this is exactly what broke before the fix).
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("returned path %q does not exist: %v", got, err)
	}
	cmd := exec.Command(got)
	cmd.Env = []string{} // deliberately empty $PATH: prove no PATH lookup is involved
	if err := cmd.Run(); err != nil {
		t.Errorf("exec.Command(%q) with empty $PATH failed: %v — EnsureBinary's returned path must be directly executable without a PATH lookup", got, err)
	}
}
