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
# Optional: use a named Cloudflare Tunnel instead of a quick/ephemeral one.
# If set, cloudflared runs as: cloudflared tunnel run --token <TUNNEL_TOKEN>
# The Public Hostname configured for that tunnel IS your domain, so also set
# WS_HOST to that hostname (e.g. reverse.example.io.vn) — it will be used
# directly instead of scraping a *.trycloudflare.com URL from the logs.
#
# Leave this blank (with WS_HOST set to a real hostname, and
# CLOUDFLARE_API_TOKEN + DOMAIN below configured) to have the app
# auto-create the named Tunnel + DNS route for you via the Cloudflare API
# instead of doing cloudflared tunnel login/create/route dns by hand — see
# CLOUDFLARE_API_TOKEN's comment below.
TUNNEL_TOKEN=%s
# Optional: auto-deploy the Cloudflare Worker bridge (script + KV binding)
# via the Cloudflare REST API instead of the dashboard, AND (when TUNNEL_TOKEN
# above is blank and WS_HOST is set to a real hostname) auto-create a named
# Cloudflare Tunnel + DNS route pointing WS_HOST at it. Set
# CLOUDFLARE_API_TOKEN (needs Workers Scripts:Edit, Workers KV Storage:Edit,
# Workers Routes:Edit (Zone), Zone:Read (Zone), Cloudflare Tunnel:Edit
# (Account), and DNS:Edit (Zone) on DOMAIN's zone) together with DOMAIN (a
# zone apex already on the same Cloudflare account, e.g. example.com — NOT
# vless.example.com, the app prepends the "vless." prefix itself for the
# Worker route) to enable — both empty (default) means this is skipped
# entirely, WEBHOOK_URL/TUNNEL_TOKEN above are used as-is.
# WS_HOST must then be a different subdomain of DOMAIN than vless.DOMAIN
# (that one's reserved for the Worker route above), e.g. tunnel.example.com.
# CLOUDFLARE_ACCOUNT_ID is only needed if the token can see more than one
# Cloudflare account. WORKER_PASSWORD is generated once below and becomes
# the deployed Worker's /setapi?password= value.
CLOUDFLARE_API_TOKEN=%s
CLOUDFLARE_ACCOUNT_ID=%s
DOMAIN=%s
WORKER_PASSWORD=%s
`
