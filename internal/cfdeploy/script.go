package cfdeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
)

// workerCompatibilityDate is pinned the same way cloudflared/xray-core
// versions are pinned elsewhere in this repo (tunnel/download.go,
// go.mod) — bump deliberately, don't float "today's date" (would silently
// change Workers runtime behavior between runs).
const workerCompatibilityDate = "2024-09-01"

type scriptBinding struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	NamespaceID string `json:"namespace_id"`
}

type scriptMetadata struct {
	MainModule        string          `json:"main_module"`
	CompatibilityDate string          `json:"compatibility_date"`
	Bindings          []scriptBinding `json:"bindings"`
}

// uploadScript deploys source as scriptName's content, bound to
// kvNamespaceID as the KV_CONFIG binding the worker.js asset expects.
// Re-uploaded every run (decision log #5) so the deployed code always
// matches this binary's embedded copy.
func uploadScript(ctx context.Context, cli *client, accountID, scriptName string, source []byte, kvNamespaceID string) error {
	meta := scriptMetadata{
		MainModule:        "worker.js",
		CompatibilityDate: workerCompatibilityDate,
		Bindings: []scriptBinding{
			{Type: "kv_namespace", Name: "KV_CONFIG", NamespaceID: kvNamespaceID},
		},
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("cfdeploy: encoding script metadata: %w", err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	metaPart, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="metadata"`},
		"Content-Type":        {"application/json"},
	})
	if err != nil {
		return fmt.Errorf("cfdeploy: creating metadata part: %w", err)
	}
	if _, err := metaPart.Write(metaJSON); err != nil {
		return fmt.Errorf("cfdeploy: writing metadata part: %w", err)
	}

	// The part name must match metadata.main_module ("worker.js") — that's
	// how the Cloudflare API pairs the metadata's module reference to the
	// actual file content in the same multipart body.
	scriptPart, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="worker.js"; filename="worker.js"`},
		"Content-Type":        {"application/javascript+module"},
	})
	if err != nil {
		return fmt.Errorf("cfdeploy: creating script part: %w", err)
	}
	if _, err := scriptPart.Write(source); err != nil {
		return fmt.Errorf("cfdeploy: writing script part: %w", err)
	}

	if err := mw.Close(); err != nil {
		return fmt.Errorf("cfdeploy: closing multipart body: %w", err)
	}

	_, err = cli.doRequest(ctx, "PUT", fmt.Sprintf("/accounts/%s/workers/scripts/%s", accountID, scriptName), &buf, mw.FormDataContentType())
	if err != nil {
		return fmt.Errorf("cfdeploy: uploading worker script %q: %w", scriptName, err)
	}
	fmt.Printf("[+] Deployed Cloudflare Worker script %q\n", scriptName)
	return nil
}
