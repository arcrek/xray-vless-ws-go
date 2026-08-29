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
	LogPassword string // optional, off (Basic Auth disabled) when empty

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
// missing key, auto-writing a default .env file if none exists yet (same UX
// as main.py's init_env_file() — first run should "just work"), then parses
// and validates every field. It never panics; malformed input is returned as
// a wrapped error naming the offending field, so main.go can log-and-exit.
func Load() (*Config, error) {
	envPath := ".env"

	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		if err := writeDefaultEnvFile(envPath); err != nil {
			return nil, fmt.Errorf("config: writing default .env: %w", err)
		}
	}

	// godotenv.Load populates process env vars from .env without
	// overwriting any that are already set (e.g. by the CI bridge or the
	// shell) — same "env wins over file" precedence main.py gets from
	// python-dotenv's load_dotenv().
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
		XrayUUID:    getenv("XRAY_UUID", newUUIDv4()),
		RawFakeSNI:  getenv("FAKE_SNI", defaultFakeSNI),
		WSPath:      getenv("WS_PATH", defaultWSPath),
		WSHost:      getenv("WS_HOST", defaultWSHost),
		Transport:   strings.ToLower(strings.TrimSpace(getenv("TRANSPORT", defaultTransport))),
		WebhookURL:  getenv("WEBHOOK_URL", defaultWebhookURL),
		TunnelToken: strings.TrimSpace(getenv("TUNNEL_TOKEN", defaultTunnelToken)),
		DebugMode:   strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_MODE"))) == "true",
		LogPassword: getenv("LOG_PASSWORD", defaultLogPassword),

		CloudflareAPIToken:  strings.TrimSpace(getenv("CLOUDFLARE_API_TOKEN", defaultCloudflareAPIToken)),
		CloudflareAccountID: strings.TrimSpace(getenv("CLOUDFLARE_ACCOUNT_ID", defaultCloudflareAccountID)),
		Domain:              strings.TrimSpace(getenv("DOMAIN", defaultDomain)),
		WorkerPassword:      getenv("WORKER_PASSWORD", newSecretToken(24)),
	}

	// TRANSPORT: only "websocket" is supported in v1. xhttp is
	// force-downgraded with a warning (kept for parity with main.py:81-86 —
	// the field stays rather than being dropped, since a user's existing
	// .env may already set it and silently ignoring the key entirely would
	// be a worse surprise than a logged downgrade).
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
		defaultTunnelToken,
		defaultCloudflareAPIToken,
		defaultCloudflareAccountID,
		defaultDomain,
		newSecretToken(24),
	)

	// 0600: .env may contain TUNNEL_TOKEN (a Cloudflare secret). The Python
	// version relies on umask; this is tightened explicitly here since
	// "production ready" is a stated goal of the rewrite.
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}

	fmt.Println("[+] Generated .env configuration.")
	return nil
}
