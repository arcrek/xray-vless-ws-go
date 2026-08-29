package cfdeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

type attachDomainRequest struct {
	Hostname string `json:"hostname"`
	Service  string `json:"service"`
	ZoneName string `json:"zone_name"`
}

// attachCustomDomain binds hostname (a subdomain of zoneName) to scriptName
// via PUT /accounts/{id}/workers/domains. Cloudflare treats re-attaching the
// same hostname/service pair as an update, not an error, matching this
// package's "always re-sync" idempotency model (decision log #5).
func attachCustomDomain(ctx context.Context, cli *client, accountID, hostname, scriptName, zoneName string) error {
	body, err := json.Marshal(attachDomainRequest{Hostname: hostname, Service: scriptName, ZoneName: zoneName})
	if err != nil {
		return fmt.Errorf("cfdeploy: encoding domain attach body: %w", err)
	}
	_, err = cli.doRequest(ctx, "PUT", fmt.Sprintf("/accounts/%s/workers/domains", accountID), bytes.NewReader(body), "")
	if err != nil {
		return fmt.Errorf("cfdeploy: attaching custom domain %q to worker %q: %w", hostname, scriptName, err)
	}
	fmt.Printf("[+] Attached Cloudflare Worker custom domain %q\n", hostname)
	return nil
}
