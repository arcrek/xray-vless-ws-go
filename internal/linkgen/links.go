package linkgen

import (
	"fmt"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

// BuildLinks builds the vless:// link list, byte for byte reproducible
// (see links_test.go's golden-fixture comparison). Every SNIEntry produces
// two links — TLS on 443, no-TLS on 80 — intentionally, not collapsed to
// one (some carriers only unmeter one port).
//
// uuid and tunnelHost are passed explicitly (rather than read off cfg)
// so callers can rebuild links after a hostname change without needing a
// fresh Config.
func BuildLinks(cfg *config.Config, uuid, tunnelHost string) []string {
	encodedPath := safeQuote(cfg.WSPath)

	// Prefer the configured WS_HOST over the detected
	// hostname whenever WS_HOST isn't left at the "trycloudflare.com"
	// default — this is the named-tunnel case, where WS_HOST is the real
	// dashboard-configured hostname and tunnelHost may just be a stand-in.
	tunnelHostInfo := tunnelHost
	if cfg.WSHost != "" && cfg.WSHost != "trycloudflare.com" {
		tunnelHostInfo = cfg.WSHost
	}

	const netType = "ws"

	payloads := make([]string, 0, len(cfg.FakeSNI)*2)
	for _, entry := range cfg.FakeSNI {
		encodedRemark := safeQuote(entry.Remark)

		payloads = append(payloads,
			fmt.Sprintf("vless://%s@%s:443?type=%s&encryption=none&security=tls&path=%s&host=%s&sni=%s#%s%%20TLS",
				uuid, entry.SNI, netType, encodedPath, tunnelHostInfo, tunnelHostInfo, encodedRemark),
			fmt.Sprintf("vless://%s@%s:80?type=%s&encryption=none&security=&path=%s&host=%s#%s%%20NO%%20TLS",
				uuid, entry.SNI, netType, encodedPath, tunnelHostInfo, encodedRemark),
		)
	}
	return payloads
}
