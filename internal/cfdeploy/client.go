// Package cfdeploy automates deploying the Cloudflare Worker bridge
// (docs/README_vi.md Bước 2 "deploy _worker.js" + Bước 3 "Workers KV
// binding") via the Cloudflare REST API, given CLOUDFLARE_API_TOKEN + DOMAIN
// in config.Config. See Ensure (deploy.go) for the entry point. This package
// depends only on internal/config, never the reverse, and is a no-op
// (returns "", nil from Ensure) whenever those two settings are absent — see
// the parent plan's decision log for the full rationale.
package cfdeploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// apiBaseURL is a var (not const) solely so deploy_test.go's
// TestEnsureHappyPath can point Ensure() at an httptest.Server instead of
// the real Cloudflare API — every other test constructs a *client directly
// with its own baseURL and never touches this.
var apiBaseURL = "https://api.cloudflare.com/client/v4"

// client is a minimal Cloudflare REST API client — just the envelope
// handling every endpoint cfdeploy calls shares. Not a general-purpose SDK
// on purpose (YAGNI): 4 endpoints (accounts, KV namespaces, worker scripts,
// worker domains) is the entire surface this package needs.
type client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

func newClient(token string) *client {
	return &client{httpClient: http.DefaultClient, baseURL: apiBaseURL, token: token}
}

// apiEnvelope mirrors Cloudflare's standard response shape
// ({success, errors[], result}) — every endpoint used here returns it.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e apiError) String() string { return fmt.Sprintf("%d: %s", e.Code, e.Message) }

// doRequest sends an HTTP request and unwraps the Cloudflare envelope,
// returning result's raw JSON on success. body may be nil (e.g. for GET);
// when body is non-nil and contentType is empty, it defaults to
// application/json (the shape every JSON-bodied call here uses — the one
// multipart caller, script.go, always passes its own contentType).
func (c *client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("cfdeploy: building request %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cfdeploy: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cfdeploy: reading response body for %s %s: %w", method, path, err)
	}

	var env apiEnvelope
	if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
		// Non-JSON body (e.g. an upstream 502 HTML page) — surface the raw
		// status + a truncated body rather than a confusing JSON-parse error.
		return nil, fmt.Errorf("cfdeploy: %s %s: HTTP %d, non-JSON body: %s", method, path, resp.StatusCode, truncate(raw, 200))
	}

	if !env.Success || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cfdeploy: %s %s: HTTP %d: %s", method, path, resp.StatusCode, formatErrors(env.Errors))
	}

	return env.Result, nil
}

func formatErrors(errs []apiError) string {
	if len(errs) == 0 {
		return "no error detail returned"
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.String()
	}
	return strings.Join(msgs, "; ")
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
