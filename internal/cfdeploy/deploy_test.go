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
	got, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no-op)", got)
	}
}

func TestEnsureNoOpWhenDomainMissing(t *testing.T) {
	cfg := &config.Config{CloudflareAPIToken: "tok"}
	got, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no-op)", got)
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

	got, err := Ensure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Ensure: unexpected error: %v", err)
	}
	want := "https://vless.example.com/setapi?password=" + url.QueryEscape("p@ss word")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
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

	got, err := Ensure(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Errorf("got %q on error, want empty", got)
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
