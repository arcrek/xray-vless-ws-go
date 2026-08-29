package cfdeploy

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

//go:embed assets/worker.js
var workerAssets embed.FS

const (
	scriptName       = "xray-vless-ws-bridge"
	kvNamespaceTitle = scriptName + "-kv"
	hostnamePrefix   = "vless"
	// passwordMarker appears in assets/worker.js already wrapped in double
	// quotes (`"__WORKER_PASSWORD__"`) — Ensure replaces the whole quoted
	// token with a properly JSON/JS-string-escaped literal of
	// cfg.WorkerPassword, so a password containing a `"` or `\` can't break
	// out of the string and corrupt the deployed script.
	passwordMarker = `"__WORKER_PASSWORD__"`
)

// Ensure provisions (or re-syncs) the Cloudflare Worker bridge — the API
// automation of docs/README_vi.md's Bước 2 (deploy _worker.js) and Bước 3
// (Workers KV binding) — and returns the WEBHOOK_URL to use.
//
// It's a no-op ("", nil) when cfg.CloudflareAPIToken or cfg.Domain is empty
// (decision log #6 in the plan): callers should fall back to
// cfg.WebhookURL exactly as if this function didn't exist. Any provisioning
// error is returned, never panics or exits — per decision log #8, the
// caller (cmd/xrayws/main.go) treats it as a non-fatal warning, the same
// "independently non-fatal" pattern the rest of the webhook delivery path
// already uses; this function must never block the tunnel from starting.
func Ensure(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.CloudflareAPIToken == "" || cfg.Domain == "" {
		return "", nil
	}

	cli := newClient(cfg.CloudflareAPIToken)

	accountID, err := resolveAccountID(ctx, cli, cfg.CloudflareAccountID)
	if err != nil {
		return "", err
	}

	kvNamespaceID, err := ensureKVNamespace(ctx, cli, accountID, kvNamespaceTitle)
	if err != nil {
		return "", err
	}

	source, err := renderWorkerSource(cfg.WorkerPassword)
	if err != nil {
		return "", err
	}

	if err := uploadScript(ctx, cli, accountID, scriptName, source, kvNamespaceID); err != nil {
		return "", err
	}

	hostname := hostnamePrefix + "." + cfg.Domain
	if err := attachCustomDomain(ctx, cli, accountID, hostname, scriptName, cfg.Domain); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%s/setapi?password=%s", hostname, url.QueryEscape(cfg.WorkerPassword)), nil
}

// renderWorkerSource reads the embedded worker.js template and substitutes
// passwordMarker with a JSON-escaped literal of password.
func renderWorkerSource(password string) ([]byte, error) {
	tmpl, err := workerAssets.ReadFile("assets/worker.js")
	if err != nil {
		return nil, fmt.Errorf("cfdeploy: reading embedded worker.js: %w", err)
	}
	return substitutePassword(tmpl, password)
}

// substitutePassword is the pure part of renderWorkerSource, split out so
// tests can exercise the missing-marker error path without needing a second
// embed.FS (the real embedded worker.js's marker presence is covered
// separately, see TestWorkerAssetHasPasswordMarker).
func substitutePassword(tmpl []byte, password string) ([]byte, error) {
	literal, err := json.Marshal(password)
	if err != nil {
		return nil, fmt.Errorf("cfdeploy: encoding worker password literal: %w", err)
	}

	if !strings.Contains(string(tmpl), passwordMarker) {
		return nil, fmt.Errorf("cfdeploy: worker.js template is missing the %s marker", passwordMarker)
	}

	return []byte(strings.ReplaceAll(string(tmpl), passwordMarker, string(literal))), nil
}
