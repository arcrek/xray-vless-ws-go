package tunnel

import (
	"fmt"
	"os"
)

// WriteNamedTunnelConfig writes config.yml for the named-tunnel path, ported
// from main.py:243-252 (write_cloudflared_config()). Named tunnels dial out
// using this file plus TUNNEL_TOKEN embedded as the `tunnel:` field — no
// hostname is ever scraped from logs for this path (verify this explicitly:
// grep the named-tunnel launch code for any log-parsing call — there is
// none, Launch() reads WS_HOST directly from config).
func WriteNamedTunnelConfig(path, tunnelToken, wsHost, targetIP string, targetPort int) error {
	content := fmt.Sprintf(
		"tunnel: %s\n\ningress:\n  - hostname: %s\n    service: http://%s:%d\n  - service: http_status:404\n",
		tunnelToken, wsHost, targetIP, targetPort,
	)
	return os.WriteFile(path, []byte(content), 0o600)
}
