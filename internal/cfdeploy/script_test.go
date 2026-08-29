package cfdeploy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUploadScriptRequestShape(t *testing.T) {
	var gotMeta scriptMetadata
	var gotScript []byte
	var gotPath, gotContentType string

	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}

		metaPart := r.MultipartForm.Value["metadata"]
		if len(metaPart) != 1 {
			t.Fatalf("expected exactly one metadata part, got %d", len(metaPart))
		}
		if err := json.Unmarshal([]byte(metaPart[0]), &gotMeta); err != nil {
			t.Fatalf("unmarshaling metadata: %v", err)
		}

		files := r.MultipartForm.File["worker.js"]
		if len(files) != 1 {
			t.Fatalf("expected exactly one worker.js file part, got %d", len(files))
		}
		f, err := files[0].Open()
		if err != nil {
			t.Fatalf("opening worker.js part: %v", err)
		}
		defer f.Close()
		buf, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("reading worker.js part: %v", err)
		}
		gotScript = buf

		w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"xray-vless-ws-bridge"}}`))
	})

	source := []byte(`export default { async fetch() { return new Response("hi"); } };`)
	err := uploadScript(context.Background(), cli, "acc1", "xray-vless-ws-bridge", source, "kv-id-123")
	if err != nil {
		t.Fatalf("uploadScript: unexpected error: %v", err)
	}

	if gotPath != "/accounts/acc1/workers/scripts/xray-vless-ws-bridge" {
		t.Errorf("path = %q, want /accounts/acc1/workers/scripts/xray-vless-ws-bridge", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data prefix", gotContentType)
	}
	if gotMeta.MainModule != "worker.js" {
		t.Errorf("MainModule = %q, want worker.js", gotMeta.MainModule)
	}
	if gotMeta.CompatibilityDate != workerCompatibilityDate {
		t.Errorf("CompatibilityDate = %q, want %q", gotMeta.CompatibilityDate, workerCompatibilityDate)
	}
	if len(gotMeta.Bindings) != 1 {
		t.Fatalf("expected exactly one binding, got %d", len(gotMeta.Bindings))
	}
	b := gotMeta.Bindings[0]
	if b.Type != "kv_namespace" || b.Name != "KV_CONFIG" || b.NamespaceID != "kv-id-123" {
		t.Errorf("binding = %+v, want {kv_namespace KV_CONFIG kv-id-123}", b)
	}
	if string(gotScript) != string(source) {
		t.Errorf("uploaded script content = %q, want %q", gotScript, source)
	}
}

func TestUploadScriptAPIErrorPropagates(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"upload failed"}]}`))
	})

	err := uploadScript(context.Background(), cli, "acc1", "name", []byte("x"), "kv-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
