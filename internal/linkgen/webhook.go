package linkgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SendWebhook POSTs payload as JSON to url, fire-and-forget on its own
// goroutine with a 10s timeout — matching main.py:108-126's
// threading.Thread(daemon=True) + requests.post(..., timeout=10). Must
// never block link printing/writing if the webhook endpoint is slow or
// unreachable, so this launches the goroutine and returns immediately; the
// caller does not (and should not) wait on it.
func SendWebhook(url string, payload any) {
	if url == "" {
		return
	}
	go func() {
		body, err := json.Marshal(payload)
		if err != nil {
			fmt.Printf("[!] Error sending webhook: %v\n", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			fmt.Printf("[!] Error sending webhook: %v\n", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("[!] Error sending webhook: %v\n", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Println("[+] Webhook sent successfully!")
		} else {
			fmt.Printf("[-] Webhook failed with status: %d\n", resp.StatusCode)
		}
	}()
}
