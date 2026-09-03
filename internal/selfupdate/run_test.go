package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func contextBG() context.Context { return context.Background() }

// newFakeReleaseServer serves a canned "releases/latest" JSON body (with a
// single platform asset + SHA256SUMS) plus the asset/sums bodies
// themselves, so latestRelease/downloadAndVerify/Run can be exercised
// end-to-end against something other than the real GitHub API.
func newFakeReleaseServer(t *testing.T, tag, assetName string, assetBody []byte) *httptest.Server {
	t.Helper()
	hash := sha256.Sum256(assetBody)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), assetName)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	mux.HandleFunc("/repos/"+githubRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"tag_name": tag,
			"assets": []map[string]string{
				{"name": assetName, "browser_download_url": srv.URL + "/asset"},
				{"name": "SHA256SUMS", "browser_download_url": srv.URL + "/sums"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { w.Write(assetBody) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sums)) })
	return srv
}

// withFakeExecutable points osExecutable at a real file under a fresh
// t.TempDir() (pre-populated with oldContents) for the duration of the
// test, and restores it afterward. Returns the fake execPath.
func withFakeExecutable(t *testing.T, oldContents string) string {
	t.Helper()
	dir := t.TempDir()
	execPath := filepath.Join(dir, "xrayws")
	if err := os.WriteFile(execPath, []byte(oldContents), 0755); err != nil {
		t.Fatal(err)
	}
	old := osExecutable
	osExecutable = func() (string, error) { return execPath, nil }
	t.Cleanup(func() { osExecutable = old })
	return execPath
}

// withFakeRestart injects fake isUnitPresent/execCommand hooks for the
// duration of the test so Run/Rollback never shell out to a real
// systemctl. present controls isUnitPresent's return; succeed controls
// whether the injected restart command exits zero.
func withFakeRestart(t *testing.T, present, succeed bool) {
	t.Helper()
	oldIsUnitPresent, oldExecCommand := isUnitPresent, execCommand
	isUnitPresent = func(ctx context.Context) bool { return present }
	execCommand = func(name string, args ...string) *exec.Cmd {
		if succeed {
			return exec.Command("true")
		}
		return exec.Command("false")
	}
	t.Cleanup(func() { isUnitPresent, execCommand = oldIsUnitPresent, oldExecCommand })
}

func TestRunSameVersionShortCircuit(t *testing.T) {
	assetName := fmt.Sprintf("xrayws-%s-%s", runtime.GOOS, runtime.GOARCH)
	downloadHit := false

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/repos/"+githubRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"tag_name": "v1.3.0",
			"assets": []map[string]string{
				{"name": assetName, "browser_download_url": srv.URL + "/asset"},
				{"name": "SHA256SUMS", "browser_download_url": srv.URL + "/sums"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) {
		downloadHit = true
		w.Write([]byte("new binary"))
	})
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		downloadHit = true
	})

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	withFakeExecutable(t, "old binary")
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	if err := Run(contextBG(), "v1.3.0", &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "Already up to date") {
		t.Errorf("output = %q, want an up-to-date message", out.String())
	}
	if downloadHit {
		t.Error("short-circuit still downloaded the asset")
	}
}

func TestRunHappyPath(t *testing.T) {
	assetName := fmt.Sprintf("xrayws-%s-%s", runtime.GOOS, runtime.GOARCH)
	newContents := []byte("new binary contents")
	srv := newFakeReleaseServer(t, "v1.3.0", assetName, newContents)
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	execPath := withFakeExecutable(t, "old binary")
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	if err := Run(contextBG(), "v1.2.0", &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newContents) {
		t.Errorf("execPath content = %q, want %q", got, newContents)
	}
	prev, err := os.ReadFile(execPath + ".prev")
	if err != nil {
		t.Fatalf("reading .prev: %v", err)
	}
	if string(prev) != "old binary" {
		t.Errorf(".prev content = %q, want %q", prev, "old binary")
	}
	if !strings.Contains(out.String(), "v1.2.0 -> v1.3.0") {
		t.Errorf("output = %q, want an updated-version message", out.String())
	}
}

func TestRunNoSystemdUnitIsCleanSuccess(t *testing.T) {
	assetName := fmt.Sprintf("xrayws-%s-%s", runtime.GOOS, runtime.GOARCH)
	srv := newFakeReleaseServer(t, "v1.3.0", assetName, []byte("new binary"))
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	withFakeExecutable(t, "old binary")
	withFakeRestart(t, false, true) // no unit present -> restartService returns errNoSystemdUnit

	var out bytes.Buffer
	if err := Run(contextBG(), "v1.2.0", &out); err != nil {
		t.Fatalf("Run should succeed cleanly with no systemd unit, got: %v", err)
	}
	if !strings.Contains(out.String(), "no systemd unit detected") {
		t.Errorf("output = %q, want a manual-restart message", out.String())
	}
}

func TestRunRestartFailsAfterSuccessfulSwap(t *testing.T) {
	assetName := fmt.Sprintf("xrayws-%s-%s", runtime.GOOS, runtime.GOARCH)
	newContents := []byte("new binary contents")
	srv := newFakeReleaseServer(t, "v1.3.0", assetName, newContents)
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	execPath := withFakeExecutable(t, "old binary")
	withFakeRestart(t, true, false) // unit present, but the restart command itself fails

	var out bytes.Buffer
	err := Run(contextBG(), "v1.2.0", &out)
	if err == nil {
		t.Fatal("expected a non-nil error when restart fails after a successful swap")
	}
	if !strings.Contains(err.Error(), "binary updated to v1.3.0") || !strings.Contains(err.Error(), "restart failed") {
		t.Errorf("error = %q, want partial-success framing (binary updated but restart failed)", err.Error())
	}

	// The swap itself must have already succeeded despite the reported error.
	got, readErr := os.ReadFile(execPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(newContents) {
		t.Errorf("execPath content = %q, want %q (swap should have completed before the restart failure)", got, newContents)
	}
}

func TestRunMissingAsset(t *testing.T) {
	// Server only ever serves an asset for a different platform name.
	srv := newFakeReleaseServer(t, "v1.3.0", "xrayws-some-other-platform", []byte("x"))
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	withFakeExecutable(t, "old binary")
	withFakeRestart(t, true, true)

	var out bytes.Buffer
	err := Run(contextBG(), "v1.2.0", &out)
	if err == nil {
		t.Fatal("expected an error when no matching platform asset exists")
	}
}

func TestRunWindowsRefusesCleanly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only meaningful on windows; the guard is a runtime.GOOS check exercised for real there")
	}
	var out bytes.Buffer
	if err := Run(contextBG(), "v1.0.0", &out); err == nil {
		t.Fatal("expected Run to refuse on windows")
	}
}
