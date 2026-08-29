package cfdeploy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testClient starts an httptest.Server running handler and returns a client
// pointed at it — every cfdeploy test uses this instead of the real
// Cloudflare API (no live network calls, same convention as
// internal/tunnel's fakecloudflared tests).
func testClient(t *testing.T, handler http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &client{httpClient: srv.Client(), baseURL: srv.URL, token: "test-token"}
}

func TestDoRequestSuccess(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want Bearer test-token", got)
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"abc"}}`))
	})

	raw, err := cli.doRequest(context.Background(), "GET", "/whatever", nil, "")
	if err != nil {
		t.Fatalf("doRequest: unexpected error: %v", err)
	}
	if string(raw) != `{"id":"abc"}` {
		t.Errorf("result = %s, want {\"id\":\"abc\"}", raw)
	}
}

func TestDoRequestAPIFailure(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"bad token"}]}`))
	})

	_, err := cli.doRequest(context.Background(), "GET", "/whatever", nil, "")
	if err == nil {
		t.Fatal("doRequest: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad token") {
		t.Errorf("error = %v, want it to mention 'bad token'", err)
	}
}

func TestDoRequestHTTPErrorStatus(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Unauthorized to access requested resource"}]}`))
	})

	_, err := cli.doRequest(context.Background(), "GET", "/whatever", nil, "")
	if err == nil {
		t.Fatal("doRequest: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want it to mention HTTP 403", err)
	}
}

func TestDoRequestNonJSONBody(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>502 Bad Gateway</html>`))
	})

	_, err := cli.doRequest(context.Background(), "GET", "/whatever", nil, "")
	if err == nil {
		t.Fatal("doRequest: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error = %v, want it to mention HTTP 502", err)
	}
}
