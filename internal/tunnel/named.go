package tunnel

import (
	"fmt"
	"os"
)

// WriteNamedTunnelConfig writes config.yml for the named-tunnel path: just
// the ingress rules. Authentication is a separate concern, handled by
// Launch() passing TUNNEL_TOKEN via the `--token` CLI flag — NOT by putting
// it in this file. cloudflared's config.yml `tunnel:` field means something
// different: a tunnel UUID/name for the legacy credentials-file (cert.pem)
// flow, and setting it to a token instead makes cloudflared try to resolve
// that value as a tunnel ID, fail to find cert.pem, and exit
// ("error parsing tunnel ID: Error locating origin cert") — this was a
// live bug here until the token path was actually exercised end-to-end.
//
// No hostname is ever scraped from logs for this path (verify this
// explicitly: grep the named-tunnel launch code for any log-parsing call —
// there is none, Launch() reads WS_HOST directly from config).
func WriteNamedTunnelConfig(path, wsHost, targetIP string, targetPort int, http2Origin bool) error {
	var originBlock string
	if http2Origin {
		originBlock = "    originRequest:\n      http2Origin: true\n"
	}
	content := fmt.Sprintf(
		"ingress:\n  - hostname: %s\n    service: http://%s:%d\n%s  - service: http_status:404\n",
		wsHost, targetIP, targetPort, originBlock,
	)
	return os.WriteFile(path, []byte(content), 0o600)
}
