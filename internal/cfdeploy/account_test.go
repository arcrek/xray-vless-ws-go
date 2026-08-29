package cfdeploy

import (
	"context"
	"net/http"
	"testing"
)

func TestResolveAccountIDOverride(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("resolveAccountID should not call the API when override is set")
	})

	got, err := resolveAccountID(context.Background(), cli, "pinned-account")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pinned-account" {
		t.Errorf("got %q, want pinned-account", got)
	}
}

func TestResolveAccountIDSingleAccount(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			t.Errorf("path = %q, want /accounts", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"acc1","name":"My Account"}]}`))
	})

	got, err := resolveAccountID(context.Background(), cli, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "acc1" {
		t.Errorf("got %q, want acc1", got)
	}
}

func TestResolveAccountIDMultipleAccountsUsesFirst(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"first","name":"A"},{"id":"second","name":"B"}]}`))
	})

	got, err := resolveAccountID(context.Background(), cli, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first" {
		t.Errorf("got %q, want first (the first account in the list)", got)
	}
}

func TestResolveAccountIDEmptyListErrors(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
	})

	_, err := resolveAccountID(context.Background(), cli, "")
	if err == nil {
		t.Fatal("expected error for empty accounts list, got nil")
	}
}

func TestResolveAccountIDAPIErrorPropagates(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"errors":[{"code":9106,"message":"Invalid API Token"}]}`))
	})

	_, err := resolveAccountID(context.Background(), cli, "")
	if err == nil {
		t.Fatal("expected error to propagate, got nil")
	}
}
