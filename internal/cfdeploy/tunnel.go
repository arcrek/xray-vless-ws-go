package cfdeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

// tunnelName is the fixed Cloudflare Tunnel name this package creates/reuses
// — same fixed-constant idiom as scriptName/kvNamespaceTitle (YAGNI: no
// env-configurable name, one tunnel per deployment).
const tunnelName = scriptName + "-tunnel"

// ensureNamedTunnel auto-provisions a Cloudflare named Tunnel + DNS route
// for cfg.WSHost, returning a fresh connector token for internal/tunnel to
// launch cloudflared with (the same TUNNEL_TOKEN a manually-created
// dashboard tunnel would give). Unlike WORKER_PASSWORD/LOG_PASSWORD, the
// token is never persisted to .env — it's cheaply re-fetchable from the
// Cloudflare API on every run, so re-resolving the tunnel by name and
// re-fetching its token each start is simpler than keeping a copy in sync.
//
// No-op ("", nil) — never an error — when:
//   - TUNNEL_TOKEN is already set in .env (never override a manually
//     configured token)
//   - WS_HOST is empty or still the quick-tunnel default
//     ("trycloudflare.com", same literal internal/linkgen already compares
//     against) — no fixed hostname was requested
//   - WS_HOST collides with the Worker bridge's own hostname
//     (hostnamePrefix + "." + DOMAIN) — that hostname is reserved by
//     attachCustomDomain above; routing a Tunnel DNS record there too would
//     conflict with it
//   - WS_HOST isn't DOMAIN itself or a subdomain of it — DOMAIN is the only
//     zone this function has been handed a token scoped to
func ensureNamedTunnel(ctx context.Context, cli *client, accountID string, cfg *config.Config) (string, error) {
	if cfg.TunnelToken != "" || cfg.WSHost == "" || cfg.WSHost == "trycloudflare.com" {
		return "", nil
	}

	if cfg.WSHost == workerHostname(cfg.Domain) {
		fmt.Printf("[!] WS_HOST %q matches the Worker bridge hostname — that hostname is reserved by the "+
			"Worker auto-deploy above; pick a different subdomain (e.g. tunnel.%s) to avoid a route conflict. "+
			"Skipping Cloudflare Tunnel auto-create.\n", cfg.WSHost, cfg.Domain)
		return "", nil
	}
	if cfg.WSHost != cfg.Domain && !strings.HasSuffix(cfg.WSHost, "."+cfg.Domain) {
		fmt.Printf("[!] WS_HOST %q is not DOMAIN %q or a subdomain of it — skipping Cloudflare Tunnel "+
			"auto-create (DNS route needs DOMAIN's zone).\n", cfg.WSHost, cfg.Domain)
		return "", nil
	}

	tunnelID, err := ensureTunnel(ctx, cli, accountID, tunnelName)
	if err != nil {
		return "", err
	}

	token, err := ensureTunnelToken(ctx, cli, accountID, tunnelID)
	if err != nil {
		return "", err
	}

	zoneID, err := resolveZoneID(ctx, cli, cfg.Domain)
	if err != nil {
		return "", err
	}

	if err := ensureDNSRoute(ctx, cli, zoneID, cfg.WSHost, tunnelID); err != nil {
		return "", err
	}

	fmt.Printf("[+] Cloudflare Tunnel %q ready for %s\n", tunnelName, cfg.WSHost)
	return token, nil
}

type cfTunnel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ensureTunnel finds an existing tunnel by name, creating one only if none
// matches — same find-or-create idempotency as ensureKVNamespace.
func ensureTunnel(ctx context.Context, cli *client, accountID, name string) (string, error) {
	q := url.Values{"name": {name}, "is_deleted": {"false"}}.Encode()
	raw, err := cli.doRequest(ctx, "GET", fmt.Sprintf("/accounts/%s/cfd_tunnel?%s", accountID, q), nil, "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: listing tunnels: %w", err)
	}
	var tunnels []cfTunnel
	if err := json.Unmarshal(raw, &tunnels); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing tunnels list: %w", err)
	}
	for _, t := range tunnels {
		if t.Name == name {
			return t.ID, nil
		}
	}

	body, err := json.Marshal(map[string]string{"name": name, "config_src": "cloudflare"})
	if err != nil {
		return "", fmt.Errorf("cfdeploy: encoding tunnel create body: %w", err)
	}
	raw, err = cli.doRequest(ctx, "POST", fmt.Sprintf("/accounts/%s/cfd_tunnel", accountID), bytes.NewReader(body), "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: creating tunnel %q: %w", name, err)
	}
	var created cfTunnel
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing tunnel create response: %w", err)
	}
	fmt.Printf("[+] Created Cloudflare Tunnel %q (%s)\n", name, created.ID)
	return created.ID, nil
}

// ensureTunnelToken fetches a fresh connector token for tunnelID — the same
// value `cloudflared tunnel token <name>` prints, callable any number of
// times (it's not one-shot), which is why ensureNamedTunnel never persists
// it anywhere.
func ensureTunnelToken(ctx context.Context, cli *client, accountID, tunnelID string) (string, error) {
	raw, err := cli.doRequest(ctx, "GET", fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/token", accountID, tunnelID), nil, "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: fetching tunnel token: %w", err)
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing tunnel token response: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("cfdeploy: tunnel token response was empty")
	}
	return token, nil
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// resolveZoneID looks up the zone ID for a zone apex domain (DOMAIN in
// .env) via GET /zones?name=. No find-or-create here — a missing zone means
// the API token can't see that zone at all, always an error, never
// something to provision.
func resolveZoneID(ctx context.Context, cli *client, domain string) (string, error) {
	raw, err := cli.doRequest(ctx, "GET", fmt.Sprintf("/zones?%s", url.Values{"name": {domain}}.Encode()), nil, "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: listing zones: %w", err)
	}
	var zones []cfZone
	if err := json.Unmarshal(raw, &zones); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing zones list: %w", err)
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("cfdeploy: zone %q not found on this Cloudflare account (or CLOUDFLARE_API_TOKEN lacks Zone:Read on it)", domain)
	}
	return zones[0].ID, nil
}

type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// ensureDNSRoute points hostname at the tunnel via a proxied CNAME to
// "<tunnelID>.cfargotunnel.com" — the same record `cloudflared tunnel route
// dns` creates. Idempotent: reuses an existing record with matching
// content/proxied as-is, patches one that's stale (e.g. left over from a
// previous tunnel), creates one if none exists.
func ensureDNSRoute(ctx context.Context, cli *client, zoneID, hostname, tunnelID string) error {
	target := tunnelID + ".cfargotunnel.com"

	q := url.Values{"type": {"CNAME"}, "name": {hostname}}.Encode()
	raw, err := cli.doRequest(ctx, "GET", fmt.Sprintf("/zones/%s/dns_records?%s", zoneID, q), nil, "")
	if err != nil {
		return fmt.Errorf("cfdeploy: listing DNS records for %q: %w", hostname, err)
	}
	var records []dnsRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("cfdeploy: parsing DNS records list: %w", err)
	}

	body, err := json.Marshal(dnsRecord{Type: "CNAME", Name: hostname, Content: target, Proxied: true})
	if err != nil {
		return fmt.Errorf("cfdeploy: encoding DNS record body: %w", err)
	}

	if len(records) == 0 {
		if _, err := cli.doRequest(ctx, "POST", fmt.Sprintf("/zones/%s/dns_records", zoneID), bytes.NewReader(body), ""); err != nil {
			return fmt.Errorf("cfdeploy: creating DNS record for %q: %w", hostname, err)
		}
		fmt.Printf("[+] Created DNS route %s -> %s\n", hostname, target)
		return nil
	}

	existing := records[0]
	if existing.Content == target && existing.Proxied {
		return nil // already correct, nothing to do
	}
	if _, err := cli.doRequest(ctx, "PUT", fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, existing.ID), bytes.NewReader(body), ""); err != nil {
		return fmt.Errorf("cfdeploy: updating DNS record for %q: %w", hostname, err)
	}
	fmt.Printf("[+] Updated DNS route %s -> %s\n", hostname, target)
	return nil
}
