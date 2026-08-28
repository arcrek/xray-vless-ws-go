package config

import (
	"fmt"
	"strconv"
	"strings"
)

// InboundAddr is one xray-core VLESS inbound listen address, parsed from the
// comma-separated PORT env var.
type InboundAddr struct {
	ListenIP string
	Port     int
}

// ParsePorts parses the PORT env var. Supported formats, ported 1:1 from
// main.py:88-99 (including its naive `split(":")` behavior — an IPv6 literal
// like "::1:8888" would split incorrectly; this is a pre-existing PoC
// limitation, preserved intentionally for behavior parity rather than fixed
// silently, per Phase 1's risk note):
//
//	"8888"              -> 0.0.0.0:8888
//	"127.0.0.1:8888"    -> 127.0.0.1:8888
//	"0.0.0.0:443,0.0.0.0:80" -> two inbounds
func ParsePorts(raw string) ([]InboundAddr, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("config: PORT is empty")
	}

	var out []InboundAddr
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		if strings.Contains(item, ":") {
			parts := strings.Split(item, ":")
			listenIP := strings.Join(parts[:len(parts)-1], ":")
			portStr := parts[len(parts)-1]
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return nil, fmt.Errorf("config: PORT entry %q: invalid port %q: %w", item, portStr, err)
			}
			out = append(out, InboundAddr{ListenIP: listenIP, Port: port})
		} else {
			port, err := strconv.Atoi(item)
			if err != nil {
				return nil, fmt.Errorf("config: PORT entry %q: not a valid bare port: %w", item, err)
			}
			out = append(out, InboundAddr{ListenIP: "0.0.0.0", Port: port})
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("config: PORT %q produced no valid entries", raw)
	}
	return out, nil
}

// TunnelTarget returns the IP/port cloudflared should dial, applying the
// main.py:101-106 rule: if the first inbound listens on the wildcard address,
// cloudflared must be pointed at 127.0.0.1 instead (it can't dial 0.0.0.0).
func TunnelTarget(ports []InboundAddr) (ip string, port int) {
	first := ports[0]
	ip, port = first.ListenIP, first.Port
	if ip == "0.0.0.0" {
		ip = "127.0.0.1"
	}
	return ip, port
}
