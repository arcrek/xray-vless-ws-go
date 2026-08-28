package linkgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ExportMeta carries the non-link fields of frp_info.json, matching
// main.py:386-393. WSHost here is intentionally the RAW detected/configured
// tunnel host — NOT the WS_HOST-preferring value BuildLinks uses internally
// for the link URLs themselves (main.py:389 writes the raw `tunnel_host`
// parameter to the "wshost" field, a different variable than the
// `tunnel_host_info` override used in the link strings — preserved here
// exactly, easy to accidentally collapse into one value during a port).
type ExportMeta struct {
	IP        string
	WSHost    string
	WSPath    string
	Transport string
	StartTime int64
}

type frpInfo struct {
	Payloads  []string `json:"payloads"`
	IP        string   `json:"ip"`
	WSHost    string   `json:"wshost"`
	WSPath    string   `json:"wspath"`
	Transport string   `json:"transport"`
	StartTime int64    `json:"start_time"`
}

// Export writes frp_info.config (newline-joined links, matching main.py's
// join — no trailing newline after the last entry) and frp_info.json
// (4-space indent, matching Python's json.dump(..., indent=4)) to
// configPath/jsonPath. The two writes are independent: a failure writing
// one does not prevent attempting the other, and both failures are
// returned joined rather than the first short-circuiting the second (this
// is the one behavior the Go port must NOT accidentally lose relative to
// Python's separate per-block try/except).
func Export(links []string, meta ExportMeta, configPath, jsonPath string) error {
	var errs []error

	configContent := strings.Join(links, "\n")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		errs = append(errs, fmt.Errorf("linkgen: writing %s: %w", configPath, err))
	} else {
		fmt.Printf("Written to %s\n", configPath)
	}

	info := frpInfo{
		Payloads:  links,
		IP:        meta.IP,
		WSHost:    meta.WSHost,
		WSPath:    meta.WSPath,
		Transport: meta.Transport,
		StartTime: meta.StartTime,
	}
	jsonBytes, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		errs = append(errs, fmt.Errorf("linkgen: marshaling %s: %w", jsonPath, err))
	} else if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		errs = append(errs, fmt.Errorf("linkgen: writing %s: %w", jsonPath, err))
	} else {
		fmt.Printf("Written to %s\n", jsonPath)
	}

	return errors.Join(errs...)
}
