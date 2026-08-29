package cfdeploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestAttachCustomDomainRequestShape(t *testing.T) {
	var gotPath string
	var gotBody attachDomainRequest

	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &gotBody); err != nil {
			t.Fatalf("unmarshaling request body: %v", err)
		}
		w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"domain-id","hostname":"vless.example.com"}}`))
	})

	err := attachCustomDomain(context.Background(), cli, "acc1", "vless.example.com", "xray-vless-ws-bridge", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/accounts/acc1/workers/domains" {
		t.Errorf("path = %q, want /accounts/acc1/workers/domains", gotPath)
	}
	want := attachDomainRequest{Hostname: "vless.example.com", Service: "xray-vless-ws-bridge", ZoneName: "example.com"}
	if gotBody != want {
		t.Errorf("body = %+v, want %+v", gotBody, want)
	}
}

func TestAttachCustomDomainAPIErrorPropagates(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"zone not found"}]}`))
	})

	err := attachCustomDomain(context.Background(), cli, "acc1", "vless.example.com", "name", "example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
