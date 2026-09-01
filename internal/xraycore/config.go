package xraycore

import (
	"encoding/json"
	"fmt"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

// BuildConfig builds the xray-core JSON config: one VLESS inbound per
// configured port, WS transport, a single freedom outbound. A WARP outbound
// is deliberately not inserted — out of scope for v1 per plan decision
// log #5.
//
// Built via map[string]any rather than typed structs: xray-core's JSON
// config shape is loosely typed on the wire (RawMessage settings per
// inbound/outbound), and a map keeps this function simple and easy to
// diff instead of fighting Go's static typing for a shape that's only
// ever marshaled, never unmarshaled, here.
func BuildConfig(cfg *config.Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("xraycore: nil config")
	}
	if len(cfg.Ports) == 0 {
		return nil, fmt.Errorf("xraycore: config has no inbound ports")
	}

	inbounds := make([]map[string]any, 0, len(cfg.Ports))
	for _, p := range cfg.Ports {
		inbounds = append(inbounds, map[string]any{
			"port":     p.Port,
			"listen":   p.ListenIP,
			"protocol": "vless",
			"sniffing": map[string]any{
				"enabled":      true,
				"destOverride": []string{"http", "tls"},
				"routeOnly":    true,
			},
			"settings": map[string]any{
				"clients": []map[string]any{
					{
						"id":    cfg.XrayUUID,
						"level": 0,
						"email": statsEmail,
					},
				},
				"decryption": "none",
			},
			"streamSettings": map[string]any{
				"network":  "ws",
				"security": "none",
				"wsSettings": map[string]any{
					"path":                cfg.WSPath,
					"headers":             map[string]string{},
					"heartbeatPeriod":     10,
					"maxEarlyData":        2048,
					"earlyDataHeaderName": "Sec-WebSocket-Protocol",
				},
			},
		})
	}

	xrayConfig := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"dns": map[string]any{
			"servers": []any{
				"localhost",
				"1.1.1.1",
				"8.8.8.8",
			},
			"queryStrategy": "UseIPv4",
			"disableCache":  false,
			"tag":           "dns-internal",
		},
		"inbounds": inbounds,
		"outbounds": []map[string]any{
			{
				"protocol": "freedom",
				"settings": map[string]any{
					"domainStrategy": "UseIPv4",
				},
			},
		},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
					"connIdle":          120,
					"downlinkOnly":      3,
					"uplinkOnly":        1,
					"handshake":         4,
				},
			},
		},
		"stats": map[string]any{},
	}

	out, err := json.MarshalIndent(xrayConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("xraycore: marshaling config: %w", err)
	}
	return out, nil
}
