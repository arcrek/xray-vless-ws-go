package cfdeploy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

func baseTunnelCfg() *config.Config {
	return &config.Config{
		CloudflareAPIToken: "tok",
		Domain:             "example.com",
		WSHost:             "tunnel.example.com",
	}
}

func TestEnsureNamedTunnelNoOpWhenTunnelTokenSet(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cfg := baseTunnelCfg()
	cfg.TunnelToken = "already-set"

	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty (should not override a manually configured TUNNEL_TOKEN)", token)
	}
}

func TestEnsureNamedTunnelNoOpWhenWSHostDefault(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cfg := baseTunnelCfg()
	cfg.WSHost = "trycloudflare.com"

	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty (quick-tunnel default WS_HOST)", token)
	}
}

func TestEnsureNamedTunnelNoOpWhenWSHostEmpty(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cfg := baseTunnelCfg()
	cfg.WSHost = ""

	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
}

func TestEnsureNamedTunnelNoOpWhenWSHostMatchesWorkerHostname(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cfg := baseTunnelCfg()
	cfg.WSHost = "vless.example.com" // hostnamePrefix + "." + Domain — reserved by the Worker

	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty (would collide with the Worker bridge hostname)", token)
	}
}

func TestEnsureNamedTunnelNoOpWhenWSHostOutsideZone(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	cfg := baseTunnelCfg()
	cfg.WSHost = "tunnel.otherdomain.com"

	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty (WS_HOST outside DOMAIN's zone)", token)
	}
}

// TestEnsureNamedTunnelCreatesNew covers the full happy path against a
// clean account: no existing tunnel by name, no existing DNS record.
func TestEnsureNamedTunnelCreatesNew(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			if got := r.URL.Query().Get("name"); got != tunnelName {
				t.Errorf("list tunnels name query = %q, want %q", got, tunnelName)
			}
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			body, _ := io.ReadAll(r.Body)
			if want := `"name":"` + tunnelName + `"`; !strings.Contains(string(body), want) {
				t.Errorf("create tunnel body = %s, want it to contain %s", body, want)
			}
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"tun1","name":"` + tunnelName + `"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel/tun1/token":
			w.Write([]byte(`{"success":true,"errors":[],"result":"fake-token-value"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			if got := r.URL.Query().Get("name"); got != "example.com" {
				t.Errorf("zones name query = %q, want example.com", got)
			}
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"zone1","name":"example.com"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone1/dns_records":
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone1/dns_records":
			body, _ := io.ReadAll(r.Body)
			if want := `"content":"tun1.cfargotunnel.com"`; !strings.Contains(string(body), want) {
				t.Errorf("create DNS record body = %s, want it to contain %s", body, want)
			}
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec1"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cfg := baseTunnelCfg()
	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fake-token-value" {
		t.Errorf("token = %q, want fake-token-value", token)
	}
}

// TestEnsureNamedTunnelReusesExisting covers a second run against an
// account that already has the tunnel and a correctly-pointed DNS record —
// no create/update calls should happen, only lookups + a fresh token fetch.
func TestEnsureNamedTunnelReusesExisting(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"tun1","name":"` + tunnelName + `"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel/tun1/token":
			w.Write([]byte(`{"success":true,"errors":[],"result":"fresh-token"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"zone1","name":"example.com"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone1/dns_records":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec1","type":"CNAME","name":"tunnel.example.com","content":"tun1.cfargotunnel.com","proxied":true}]}`))
		default:
			t.Fatalf("unexpected request (expected no create/update calls): %s %s", r.Method, r.URL.Path)
		}
	})

	cfg := baseTunnelCfg()
	token, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fresh-token" {
		t.Errorf("token = %q, want fresh-token", token)
	}
}

// TestEnsureNamedTunnelPatchesStaleDNSRecord covers a DNS record that
// exists but points at a different (e.g. previously deleted/recreated)
// tunnel — it must be updated in place, not left stale or duplicated.
func TestEnsureNamedTunnelPatchesStaleDNSRecord(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"tun1","name":"` + tunnelName + `"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel/tun1/token":
			w.Write([]byte(`{"success":true,"errors":[],"result":"tok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"zone1","name":"example.com"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone1/dns_records":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"rec1","type":"CNAME","name":"tunnel.example.com","content":"stale-tunnel-id.cfargotunnel.com","proxied":true}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/zones/zone1/dns_records/rec1":
			body, _ := io.ReadAll(r.Body)
			if want := `"content":"tun1.cfargotunnel.com"`; !strings.Contains(string(body), want) {
				t.Errorf("update DNS record body = %s, want it to contain %s", body, want)
			}
			w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"rec1"}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cfg := baseTunnelCfg()
	if _, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNamedTunnelZoneNotFoundErrors(t *testing.T) {
	cli := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel":
			w.Write([]byte(`{"success":true,"errors":[],"result":[{"id":"tun1","name":"` + tunnelName + `"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acc1/cfd_tunnel/tun1/token":
			w.Write([]byte(`{"success":true,"errors":[],"result":"tok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"errors":[],"result":[]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	cfg := baseTunnelCfg()
	_, err := ensureNamedTunnel(context.Background(), cli, "acc1", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
