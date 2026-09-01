// Package config loads and validates the .env-based configuration shared by
// every other package. It has zero dependency on any sibling internal
// package (xraycore/tunnel/logserver/ci) — everything else depends on
// config, never the reverse. Config is constructed once (config.Load) and
// passed by pointer to every other package's constructor; there is no
// package-level mutable state, since Phase 6's CI bridge needs to reload
// config after ENV_CONFIG is written mid-process.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds every .env-derived setting used by the application.
// ENABLE_WARP is intentionally absent — dropped per plan decision log #5.
type Config struct {
	RawPort    string
	Ports      []InboundAddr
	TargetIP   string
	TargetPort int

	XrayUUID string

	RawFakeSNI string
	FakeSNI    []SNIEntry

	WSPath string
	WSHost string

	Transport string

	WebhookURL string

	TunnelToken string // trimmed; empty means "no named tunnel"

	DebugMode   bool
	LogPassword string // fromEnv() always fills this (random if unset in .env) — see fromEnv; empty here only if a caller builds Config directly, bypassing Load

	// Cloudflare Worker auto-deploy (internal/cfdeploy). Both
	// CloudflareAPIToken and Domain must be non-empty to activate the
	// feature — see cfdeploy.Ensure. CloudflareAccountID is an optional
	// override; empty means "auto-resolve via GET /accounts".
	CloudflareAPIToken  string // trimmed; empty means auto-deploy is off
	CloudflareAccountID string
	Domain              string // trimmed; empty means auto-deploy is off
	WorkerPassword      string // baked into the deployed Worker's /setapi?password= check
}

// Load reads .env (godotenv), falling back to hardcoded defaults for any
// missing key, auto-writing a default .env file if none exists yet (first
// run should "just work"), then parses and validates every field. It never
// panics; malformed input is returned as a wrapped error naming the
// offending field, so main.go can log-and-exit.
func Load() (*Config, error) {
	envPath := ".env"

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := writeDefaultEnvFile(envPath); err != nil {
			return nil, fmt.Errorf("config: writing default .env: %w", err)
		}
	}

	// godotenv.Load populates process env vars from .env without
	// overwriting any that are already set (e.g. by the CI bridge or the
	// shell) — "env wins over file" precedence.
	if err := godotenv.Load(envPath); err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", envPath, err)
	}

	return fromEnv()
}

// Reload re-reads config after .env has been rewritten in place (used by the
// CI bridge, Phase 6, after ENV_CONFIG is exported to .env mid-process).
func Reload() (*Config, error) {
	if err := godotenv.Overload(".env"); err != nil {
		return nil, fmt.Errorf("config: reloading .env: %w", err)
	}
	return fromEnv()
}

func fromEnv() (*Config, error) {
	cfg := &Config{
		RawPort:     getenv("PORT", defaultPort),
		XrayUUID:    getenvOrGenerate("XRAY_UUID", newUUIDv4),
		RawFakeSNI:  getenv("FAKE_SNI", defaultFakeSNI),
		WSPath:      getenv("WS_PATH", defaultWSPath),
		WSHost:      getenv("WS_HOST", defaultWSHost),
		Transport:   strings.ToLower(strings.TrimSpace(getenv("TRANSPORT", defaultTransport))),
		WebhookURL:  getenv("WEBHOOK_URL", defaultWebhookURL),
		TunnelToken: strings.TrimSpace(getenv("TUNNEL_TOKEN", defaultTunnelToken)),
		DebugMode:   strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_MODE"))) == "true",
		// Falls back to a freshly generated random secret, not
		// defaultLogPassword (""), on every load where LOG_PASSWORD is
		// absent/empty — matching WORKER_PASSWORD's pattern below. Critical
		// for pre-dashboard .env files (LOG_PASSWORD never existed as a
		// key): without this, an existing deployment upgrading to the
		// dashboard feature would hit main.go's new "LOG_PASSWORD required"
		// fatal check on its very first restart. Same caveat as
		// WORKER_PASSWORD: unless the generated value is written back to
		// .env, it re-randomizes on every restart (harmless for the
		// process's own listener, but invalidates any previously
		// bookmarked dashboard credentials).
		LogPassword: getenvOrGenerate("LOG_PASSWORD", func() string { return newSecretToken(24) }),

		CloudflareAPIToken:  strings.TrimSpace(getenv("CLOUDFLARE_API_TOKEN", defaultCloudflareAPIToken)),
		CloudflareAccountID: strings.TrimSpace(getenv("CLOUDFLARE_ACCOUNT_ID", defaultCloudflareAccountID)),
		Domain:              strings.TrimSpace(getenv("DOMAIN", defaultDomain)),
		WorkerPassword:      getenvOrGenerate("WORKER_PASSWORD", func() string { return newSecretToken(24) }),
	}

	// TRANSPORT: only "websocket" is supported in v1. xhttp is
	// force-downgraded with a warning — the field stays rather than being
	// dropped, since a user's existing .env may already set it and
	// silently ignoring the key entirely would be a worse surprise than a
	// logged downgrade.
	switch cfg.Transport {
	case "websocket":
		// ok
	case "xhttp":
		fmt.Fprintln(os.Stderr, "[!] TRANSPORT=xhttp is temporarily disabled, falling back to 'websocket'.")
		cfg.Transport = "websocket"
	default:
		fmt.Fprintf(os.Stderr, "[!] Unknown TRANSPORT %q, falling back to 'websocket'.\n", cfg.Transport)
		cfg.Transport = "websocket"
	}

	if !strings.HasPrefix(cfg.WSPath, "/") {
		cfg.WSPath = "/" + cfg.WSPath
	}

	ports, err := ParsePorts(cfg.RawPort)
	if err != nil {
		return nil, err
	}
	cfg.Ports = ports
	cfg.TargetIP, cfg.TargetPort = TunnelTarget(ports)

	sni, err := ParseSNIList(cfg.RawFakeSNI)
	if err != nil {
		return nil, err
	}
	cfg.FakeSNI = sni

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// getenvOrGenerate is like getenv but also falls back when key is present
// with an empty value — the installer writes e.g. "XRAY_UUID=" verbatim
// when the user accepts a "blank = auto-generate" prompt, which os.LookupEnv
// reports as present (ok=true, value ""), not absent. Only used for the
// handful of fields whose documented default IS a freshly generated value
// (XRAY_UUID, LOG_PASSWORD, WORKER_PASSWORD) — other fields treat an
// explicit blank as a real, meaningful value (e.g. WEBHOOK_URL="" disabled).
func getenvOrGenerate(key string, generate func() string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return generate()
}

func writeDefaultEnvFile(path string) error {
	fmt.Println("[*] File .env does not exist. Using default configuration...")

	content := fmt.Sprintf(envFileTemplate,
		defaultPort,
		newUUIDv4(),
		defaultFakeSNI,
		defaultWSPath,
		defaultWSHost,
		defaultTransport,
		defaultWebhookURL,
		newSecretToken(24), // LOG_PASSWORD
		defaultTunnelToken,
		defaultCloudflareAPIToken,
		defaultCloudflareAccountID,
		defaultDomain,
		newSecretToken(24),
	)

	// 0600: .env may contain TUNNEL_TOKEN (a Cloudflare secret) — locked
	// down explicitly rather than relying on umask, since "production
	// ready" is a stated goal.
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}

	fmt.Println("[+] Generated .env configuration.")
	return nil
}
