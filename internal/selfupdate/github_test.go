package selfupdate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestRelease(t *testing.T) {
	body := struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}{
		TagName: "v1.3.0",
	}
	body.Assets = append(body.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{Name: "xrayws-linux-amd64", BrowserDownloadURL: "https://example.com/xrayws-linux-amd64"})
	body.Assets = append(body.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{Name: "SHA256SUMS", BrowserDownloadURL: "https://example.com/SHA256SUMS"})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+githubRepo+"/releases/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	tag, assets, err := latestRelease(context.Background())
	if err != nil {
		t.Fatalf("latestRelease: %v", err)
	}
	if tag != "v1.3.0" {
		t.Errorf("tag = %q, want v1.3.0", tag)
	}
	a, ok := assets["xrayws-linux-amd64"]
	if !ok {
		t.Fatalf("missing xrayws-linux-amd64 asset")
	}
	if a.DownloadURL != "https://example.com/xrayws-linux-amd64" {
		t.Errorf("download URL = %q", a.DownloadURL)
	}
	if _, ok := assets["SHA256SUMS"]; !ok {
		t.Errorf("missing SHA256SUMS asset")
	}
}

func TestLatestReleaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	if _, _, err := latestRelease(context.Background()); err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}
