package cfdeploy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestEnsureKVNamespaceReusesExisting(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET only (no create call), got %s", r.Method)
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[
			{"id":"other-id","title":"other-title"},
			{"id":"match-id","title":"xray-vless-ws-bridge-kv"}
		]}`))
	})

	got, err := ensureKVNamespace(context.Background(), cli, "acc1", "xray-vless-ws-bridge-kv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "match-id" {
		t.Errorf("got %q, want match-id (reused, not recreated)", got)
	}
}

func TestEnsureKVNamespaceCreatesWhenMissing(t *testing.T) {
	var sawCreateBody string
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			sawCreateBody = string(b)
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"new-id","title":"xray-vless-ws-bridge-kv"}}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})

	got, err := ensureKVNamespace(context.Background(), cli, "acc1", "xray-vless-ws-bridge-kv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "new-id" {
		t.Errorf("got %q, want new-id", got)
	}
	if !strings.Contains(sawCreateBody, `"title":"xray-vless-ws-bridge-kv"`) {
		t.Errorf("create body = %q, want it to contain the title", sawCreateBody)
	}
}

func TestEnsureKVNamespaceListErrorPropagates(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"boom"}]}`))
	})

	_, err := ensureKVNamespace(context.Background(), cli, "acc1", "title")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureKVNamespaceCreateErrorPropagates(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"errors":[{"code":2,"message":"create failed"}]}`))
	})

	_, err := ensureKVNamespace(context.Background(), cli, "acc1", "title")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
