package config

import (
	"fmt"
	"strings"
)

// SNIEntry is one FAKE_SNI list entry: the SNI/hostname clients present, plus
// an optional human-readable remark used in link labels. Structured here (not
// left as a raw string) so Phase 4's link generator doesn't duplicate this
// parsing.
type SNIEntry struct {
	SNI    string
	Remark string
}

// ParseSNIList parses the FAKE_SNI env var, ported from main.py:355-365.
// Each comma-separated entry may carry a "#remark" suffix; a bare entry (or
// an entry with an empty remark after "#") gets a default "Tunnel N" remark.
func ParseSNIList(raw string) ([]SNIEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("config: FAKE_SNI is empty")
	}

	parts := strings.Split(raw, ",")
	out := make([]SNIEntry, 0, len(parts))
	for idx, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		var sni, remark string
		if strings.Contains(item, "#") {
			split := strings.SplitN(item, "#", 2)
			sni = strings.TrimSpace(split[0])
			remark = strings.TrimSpace(split[1])
			if remark == "" {
				remark = fmt.Sprintf("Tunnel %d", idx+1)
			}
		} else {
			sni = item
			remark = fmt.Sprintf("Tunnel %d", idx+1)
		}

		if sni == "" {
			return nil, fmt.Errorf("config: FAKE_SNI entry %q has an empty SNI", item)
		}
		out = append(out, SNIEntry{SNI: sni, Remark: remark})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("config: FAKE_SNI %q produced no valid entries", raw)
	}
	return out, nil
}
