package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndVerifySuccess(t *testing.T) {
	assetBody := []byte("fake binary contents")
	hash := sha256.Sum256(assetBody)
	sums := fmt.Sprintf("%s  xrayws-linux-amd64\n", hex.EncodeToString(hash[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.Write(assetBody)
		case "/sums":
			w.Write([]byte(sums))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "xrayws-update-tmp")
	err := downloadAndVerify(contextBG(), srv.URL+"/asset", srv.URL+"/sums", "xrayws-linux-amd64", dest)
	if err != nil {
		t.Fatalf("downloadAndVerify: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != string(assetBody) {
		t.Errorf("downloaded content mismatch")
	}
}

func TestDownloadAndVerifyMismatch(t *testing.T) {
	assetBody := []byte("fake binary contents")
	wrongHash := sha256.Sum256([]byte("different contents"))
	sums := fmt.Sprintf("%s  xrayws-linux-amd64\n", hex.EncodeToString(wrongHash[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.Write(assetBody)
		case "/sums":
			w.Write([]byte(sums))
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "xrayws-update-tmp")
	err := downloadAndVerify(contextBG(), srv.URL+"/asset", srv.URL+"/sums", "xrayws-linux-amd64", dest)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
}

func TestDownloadAndVerifyMissingSumsEntry(t *testing.T) {
	assetBody := []byte("fake binary contents")
	sums := "deadbeef  some-other-file\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset":
			w.Write(assetBody)
		case "/sums":
			w.Write([]byte(sums))
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "xrayws-update-tmp")
	err := downloadAndVerify(contextBG(), srv.URL+"/asset", srv.URL+"/sums", "xrayws-linux-amd64", dest)
	if err == nil {
		t.Fatal("expected error for missing SHA256SUMS entry, got nil")
	}
}
