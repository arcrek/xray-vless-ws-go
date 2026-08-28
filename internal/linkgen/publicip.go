package linkgen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LookupPublicIP wraps the api.ipify.org call from main.py:41-48
// (get_public_url()) with a timeout, falling back to "0.0.0.0" on any
// failure — matching the Python version's already-non-fatal behavior, kept
// that way here since a link export must never block on this.
func LookupPublicIP(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		fmt.Printf("[!] Failed to get public IP: %v\n", err)
		return "0.0.0.0"
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[!] Failed to get public IP: %v\n", err)
		return "0.0.0.0"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("[!] Failed to get public IP: %v\n", err)
		return "0.0.0.0"
	}
	return string(body)
}
