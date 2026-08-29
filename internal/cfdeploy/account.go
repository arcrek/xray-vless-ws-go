package cfdeploy

import (
	"context"
	"encoding/json"
	"fmt"
)

type accountInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// resolveAccountID returns override verbatim if non-empty (the
// CLOUDFLARE_ACCOUNT_ID escape hatch); otherwise resolves it via
// GET /accounts, using the first result when the token can see more than
// one account and warning the user to pin CLOUDFLARE_ACCOUNT_ID in that
// case (decision log #2).
func resolveAccountID(ctx context.Context, cli *client, override string) (string, error) {
	if override != "" {
		return override, nil
	}

	raw, err := cli.doRequest(ctx, "GET", "/accounts", nil, "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: listing accounts: %w", err)
	}

	var accounts []accountInfo
	if err := json.Unmarshal(raw, &accounts); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing accounts list: %w", err)
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("cfdeploy: CLOUDFLARE_API_TOKEN can't see any accounts")
	}
	if len(accounts) > 1 {
		fmt.Printf("[!] Cloudflare token can see %d accounts, using %q (%s) — set CLOUDFLARE_ACCOUNT_ID to pin a specific one.\n",
			len(accounts), accounts[0].Name, accounts[0].ID)
	}
	return accounts[0].ID, nil
}
