package cfdeploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

type kvNamespace struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ensureKVNamespace finds an existing namespace by title, creating one only
// if none matches — the namespace (and any config already stored in it via
// the Worker's KV_CONFIG binding) is never re-created on later runs
// (decision log #5), so data survives across restarts/redeploys.
//
// Limitation: only the first page of GET .../kv/namespaces is inspected (no
// result_info pagination). Fine for the handful of namespaces a personal
// account realistically has; an account with 20+ namespaces already using
// this exact title further down the list would cause a duplicate to be
// created instead of reused.
func ensureKVNamespace(ctx context.Context, cli *client, accountID, title string) (string, error) {
	raw, err := cli.doRequest(ctx, "GET", fmt.Sprintf("/accounts/%s/storage/kv/namespaces", accountID), nil, "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: listing KV namespaces: %w", err)
	}
	var namespaces []kvNamespace
	if err := json.Unmarshal(raw, &namespaces); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing KV namespaces list: %w", err)
	}
	for _, ns := range namespaces {
		if ns.Title == title {
			return ns.ID, nil
		}
	}

	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return "", fmt.Errorf("cfdeploy: encoding KV namespace create body: %w", err)
	}
	raw, err = cli.doRequest(ctx, "POST", fmt.Sprintf("/accounts/%s/storage/kv/namespaces", accountID), bytes.NewReader(body), "")
	if err != nil {
		return "", fmt.Errorf("cfdeploy: creating KV namespace %q: %w", title, err)
	}
	var created kvNamespace
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("cfdeploy: parsing KV namespace create response: %w", err)
	}
	fmt.Printf("[+] Created Cloudflare KV namespace %q (%s)\n", title, created.ID)
	return created.ID, nil
}
