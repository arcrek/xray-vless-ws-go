package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ReadyState is one observation of cloudflared's /ready endpoint.
type ReadyState struct {
	Ready            bool
	ReadyConnections int
	Err              error // non-nil on connection-refused / non-200-or-503 / decode failure
}

// readyResponse matches the verified /ready JSON shape:
// {"status":200,"readyConnections":1,"connectorId":"..."}.
type readyResponse struct {
	Status           int    `json:"status"`
	ReadyConnections int    `json:"readyConnections"`
	ConnectorID      string `json:"connectorId"`
}

// pollReadyOnce performs a single /ready check against metricsAddr.
func pollReadyOnce(ctx context.Context, client *http.Client, metricsAddr string) ReadyState {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+metricsAddr+"/ready", nil)
	if err != nil {
		return ReadyState{Err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		// Connection-refused (metrics server not up yet) is the common,
		// expected case right after launch — not-ready, not a hard error.
		return ReadyState{Ready: false}
	}
	defer resp.Body.Close()

	var body readyResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&body); decErr != nil {
		return ReadyState{Err: fmt.Errorf("tunnel: decoding /ready response: %w", decErr)}
	}

	// Per the verified cloudflared metrics router: /ready returns 200 when
	// connected, 503 otherwise. Any other status is treated as not-ready
	// rather than erroring, matching main.py's original tolerance for
	// unexpected responses during startup.
	return ReadyState{Ready: resp.StatusCode == http.StatusOK, ReadyConnections: body.ReadyConnections}
}

// PollReady polls /ready on metricsAddr every interval until ctx is
// cancelled, emitting a ReadyState on the returned channel each time. The
// channel is closed when ctx is done. Replaces main.py's blind
// time.sleep(1) poll loop (main.py:400-416) with an actual connectivity
// signal for Phase 3's Supervisor to act on.
func PollReady(ctx context.Context, metricsAddr string, interval time.Duration) <-chan ReadyState {
	out := make(chan ReadyState, 1)
	client := &http.Client{Timeout: 3 * time.Second}

	go func() {
		defer close(out)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := pollReadyOnce(ctx, client, metricsAddr)
				select {
				case out <- state:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}
