package config

// Default values (no ENABLE_WARP key — dropped per plan decision log #5,
// WARP is out of scope for v1).
const (
	defaultPort        = "127.0.0.1:8888"
	defaultFakeSNI     = "api24-normal-alisg.tiktokv.com#Tiktok"
	defaultWSPath      = "/tiktok4g"
	defaultWSHost      = "trycloudflare.com"
	defaultTransport   = "websocket"
	defaultWebhookURL  = ""
	defaultTunnelToken = ""
	// No defaultLogPassword const — fromEnv() falls back to a freshly
	// generated newSecretToken(24), not a fixed empty default (config.go).

	defaultCloudflareAPIToken  = ""
	defaultCloudflareAccountID = ""
	defaultDomain              = ""
)

// envFileTemplate is written verbatim (with XRAY_UUID substituted) when no
// .env file exists yet, so first run "just works".
const envFileTemplate = `PORT=%s
XRAY_UUID=%s
FAKE_SNI=%s
WS_PATH=%s
WS_HOST=%s
TRANSPORT=%s
WEBHOOK_URL=%s
# Required whenever the embedded dashboard/log viewer is enabled (the
# default: --log-port=9999). Generated randomly on first run so a fresh
# install works out of the box — the dashboard serves live traffic stats
# and the tunnel hostname, so it refuses to start unauthenticated. Change
# this, or pass --log-port=0 to disable the dashboard entirely.
LOG_PASSWORD=%s
# Optional: use a named Cloudflare Tunnel (Zero Trust dashboard) instead of a
# quick/ephemeral tunnel. If set, cloudflared runs as:
#   cloudflared tunnel run --token <TUNNEL_TOKEN>
# The Public Hostname you configured on the dashboard IS your domain, so also
# set WS_HOST to that hostname (e.g. reverse.example.io.vn) — it will be used
# directly instead of scraping a *.trycloudflare.com URL from the logs.
TUNNEL_TOKEN=%s
# Optional: auto-deploy the Cloudflare Worker bridge (script + KV binding)
# via the Cloudflare REST API instead of the dashboard. Set
# CLOUDFLARE_API_TOKEN (needs Workers Scripts:Edit, Workers KV Storage:Edit,
# Workers Routes:Edit (Zone), and Zone:Read (Zone) on DOMAIN's zone) together
# with DOMAIN (a zone apex already on the same Cloudflare account, e.g.
# example.com — NOT vless.example.com, the app prepends the "vless." prefix
# itself) to enable — both empty (default) means this is skipped entirely,
# WEBHOOK_URL above is used as-is. CLOUDFLARE_ACCOUNT_ID is only needed if
# the token can see more than one Cloudflare account. WORKER_PASSWORD is
# generated once below and becomes the deployed Worker's /setapi?password=
# value.
CLOUDFLARE_API_TOKEN=%s
CLOUDFLARE_ACCOUNT_ID=%s
DOMAIN=%s
WORKER_PASSWORD=%s
`
