package config

// Default values, matching xray_vless_ws_server/.env.example (minus ENABLE_WARP,
// which is dropped per plan decision log #5 — WARP is out of scope for v1).
const (
	defaultPort        = "127.0.0.1:8888"
	defaultFakeSNI     = "api24-normal-alisg.tiktokv.com#Tiktok,vnpt.theworkpc.com#Free VNPT"
	defaultWSPath      = "/tiktok4g"
	defaultWSHost      = "trycloudflare.com"
	defaultTransport   = "websocket"
	defaultWebhookURL  = ""
	defaultTunnelToken = ""
	defaultLogPassword = ""
)

// envFileTemplate is written verbatim (with XRAY_UUID substituted) when no
// .env file exists yet, mirroring main.py's init_env_file().
const envFileTemplate = `PORT=%s
XRAY_UUID=%s
FAKE_SNI=%s
WS_PATH=%s
WS_HOST=%s
TRANSPORT=%s
WEBHOOK_URL=%s
# Optional: use a named Cloudflare Tunnel (Zero Trust dashboard) instead of a
# quick/ephemeral tunnel. If set, cloudflared runs as:
#   cloudflared tunnel run --token <TUNNEL_TOKEN>
# The Public Hostname you configured on the dashboard IS your domain, so also
# set WS_HOST to that hostname (e.g. reverse.example.io.vn) — it will be used
# directly instead of scraping a *.trycloudflare.com URL from the logs.
TUNNEL_TOKEN=%s
`
