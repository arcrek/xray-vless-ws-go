package cfdeploy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

func TestEnsureNoOpWhenTokenMissing(t *testing.T) {
	cfg := &config.Config{Domain: "example.com"}
	webhookURL, tunnelToken, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if webhookURL != "" || tunnelToken != "" {
		t.Errorf("got (%q, %q), want (\"\", \"\") (no-op)", webhookURL, tunnelToken)
	}
}

func TestEnsureNoOpWhenDomainMissing(t *testing.T) {
	cfg := &config.Config{CloudflareAPIToken: "tok"}
	webhookURL, tunnelToken, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if webhookURL != "" || tunnelToken != "" {
		t.Errorf("got (%q, %q), want (\"\", \"\") (no-op)", webhookURL, tunnelToken)
	}
}

func TestEnsureHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"acc1","name":"Test"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/storage/kv/namespaces":
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acc1/storage/kv/namespaces":
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"kv1","title":"xray-vless-ws-bridge-kv"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acc1/workers/scripts/xray-vless-ws-bridge":
			io.ReadAll(r.Body) // drain, don't care about shape here (covered by TestUploadScriptRequestShape)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"xray-vless-ws-bridge"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acc1/workers/domains":
			io.ReadAll(r.Body)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"d1","hostname":"vless.example.com"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	old := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = old })

	cfg := &config.Config{
		CloudflareAPIToken: "tok",
		Domain:             "example.com",
		WorkerPassword:     "p@ss word",
	}

	webhookURL, tunnelToken, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ensure: unexpected error: %v", err)
	}
	want := "https://vless.example.com/setapi?password=" + url.QueryEscape("p@ss word")
	if webhookURL != want {
		t.Errorf("webhookURL = %q, want %q", webhookURL, want)
	}
	// cfg.WSHost is unset in this test, so ensureNamedTunnel is a no-op and
	// no cfd_tunnel/zones/dns_records requests are expected — the handler
	// above would t.Fatalf on any unhandled path if it made one.
	if tunnelToken != "" {
		t.Errorf("tunnelToken = %q, want empty (WS_HOST unset, Tunnel auto-create should no-op)", tunnelToken)
	}
}

// TestEnsureKeepsWebhookURLWhenTunnelStepFails is the regression case for a
// bug found in code review: the Worker deploy (KV + script + custom domain)
// fully succeeds, but the later, independent Tunnel auto-provision step
// errors (here: the zone lookup returns none) — Ensure must still return
// the already-successful webhookURL instead of discarding it alongside the
// tunnel error.
func TestEnsureKeepsWebhookURLWhenTunnelStepFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"acc1","name":"Test"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/storage/kv/namespaces":
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acc1/storage/kv/namespaces":
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"kv1","title":"xray-vless-ws-bridge-kv"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acc1/workers/scripts/xray-vless-ws-bridge":
			io.ReadAll(r.Body)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"xray-vless-ws-bridge"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acc1/workers/domains":
			io.ReadAll(r.Body)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"d1","hostname":"vless.example.com"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			io.ReadAll(r.Body)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"tun1","name":"xray-vless-ws-bridge-tunnel"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel/tun1/token":
			w.Write([]byte(`{"success":true,"errors":[],"result":"tok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			// Zone not found — the Tunnel step fails here, after the
			// Worker deploy above has already fully succeeded.
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	old := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = old })

	cfg := &config.Config{
		CloudflareAPIToken: "tok",
		Domain:             "example.com",
		WorkerPassword:     "p",
		WSHost:             "tunnel.example.com",
	}

	webhookURL, tunnelToken, err := Ensure(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error from the tunnel step (zone not found), got nil")
	}
	want := "https://vless.example.com/setapi?password=p"
	if webhookURL != want {
		t.Errorf("webhookURL = %q, want %q (must survive a later tunnel-step failure)", webhookURL, want)
	}
	if tunnelToken != "" {
		t.Errorf("tunnelToken = %q, want empty on error", tunnelToken)
	}
}

func TestEnsureErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"errors":[{"code":9106,"message":"Invalid API Token"}]}`))
	}))
	defer srv.Close()

	old := apiBaseURL
	apiBaseURL = srv.URL
	t.Cleanup(func() { apiBaseURL = old })

	cfg := &config.Config{CloudflareAPIToken: "bad-tok", Domain: "example.com", WorkerPassword: "x"}

	webhookURL, tunnelToken, err := Ensure(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if webhookURL != "" || tunnelToken != "" {
		t.Errorf("got (%q, %q) on error, want (\"\", \"\")", webhookURL, tunnelToken)
	}
}

func TestRenderWorkerSourceSubstitutesAndEscapes(t *testing.T) {
	src, err := renderWorkerSource(`p"ss\word`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(src)
	if strings.Contains(s, passwordMarker) {
		t.Error("marker was not substituted")
	}
	// json.Marshal of `p"ss\word` yields `"p\"ss\\word"` — the escaped form
	// must appear verbatim in the rendered source (a real JS string literal
	// the marker's surrounding quotes are replaced by, whole).
	if !strings.Contains(s, `"p\"ss\\word"`) {
		t.Errorf("rendered source does not contain the JSON-escaped password literal:\n%s", s)
	}
}

func TestSubstitutePasswordMissingMarkerErrors(t *testing.T) {
	_, err := substitutePassword([]byte("no marker in here"), "pw")
	if err == nil {
		t.Fatal("expected error for a template missing the password marker, got nil")
	}
}

// TestWorkerAssetHasPasswordMarker is a regression guard: if someone edits
// assets/worker.js and drops the marker, renderWorkerSource would fail at
// runtime for every real deploy — catch that here instead.
func TestWorkerAssetHasPasswordMarker(t *testing.T) {
	tmpl, err := workerAssets.ReadFile("assets/worker.js")
	if err != nil {
		t.Fatalf("reading embedded worker.js: %v", err)
	}
	if !strings.Contains(string(tmpl), passwordMarker) {
		t.Fatalf("assets/worker.js is missing %s", passwordMarker)
	}
}
