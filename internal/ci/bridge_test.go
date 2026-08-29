package ci

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDispatchWorkflowSuccess(t *testing.T) {
	var dispatchedRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/actions/runs/42":
			json.NewEncoder(w).Encode(map[string]any{"workflow_id": 999})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/actions/workflows/999/dispatches":
			var body struct {
				Ref string `json:"ref"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			dispatchedRef = body.Ref
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	os.Setenv("GITHUB_REPOSITORY", "owner/repo")
	os.Setenv("GITHUB_REF", "refs/heads/main")
	os.Setenv("GITHUB_RUN_ID", "42")
	os.Setenv("GITHUB_EVENT_PATH", "")
	defer os.Unsetenv("GITHUB_REPOSITORY")
	defer os.Unsetenv("GITHUB_REF")
	defer os.Unsetenv("GITHUB_RUN_ID")
	defer os.Unsetenv("GITHUB_EVENT_PATH")

	if err := dispatchWorkflow(context.Background(), "tok"); err != nil {
		t.Fatalf("dispatchWorkflow: %v", err)
	}
	if dispatchedRef != "main" {
		t.Errorf("dispatched ref = %q, want %q (branch extracted from refs/heads/main)", dispatchedRef, "main")
	}
}

func TestDispatchWorkflowRunFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	oldBase := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = oldBase }()

	os.Setenv("GITHUB_REPOSITORY", "owner/repo")
	os.Setenv("GITHUB_REF", "refs/heads/main")
	os.Setenv("GITHUB_RUN_ID", "42")
	defer os.Unsetenv("GITHUB_REPOSITORY")
	defer os.Unsetenv("GITHUB_REF")
	defer os.Unsetenv("GITHUB_RUN_ID")

	if err := dispatchWorkflow(context.Background(), "tok"); err == nil {
		t.Fatal("expected an error when the run-fetch request 404s, got nil")
	}
}
