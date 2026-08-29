package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const bridgeDelay = 5 * time.Hour

// githubAPIBase is overridable in tests so dispatchWorkflow/fetchWorkflowID
// can be exercised against an httptest.Server instead of the real GitHub API.
var githubAPIBase = "https://api.github.com"

// ScheduleBridge, if BRIDGE_WORKFLOWS=true, schedules a self re-dispatch of
// the current GitHub Actions workflow at +5h to continue past the 6h job
// time limit. It returns immediately; the actual dispatch happens on its
// own goroutine after bridgeDelay, or not at all if ctx is cancelled first
// (e.g. clean shutdown before the delay elapses).
func ScheduleBridge(ctx context.Context, log LogFunc) {
	if log == nil {
		log = func(string) {}
	}
	if strings.ToLower(os.Getenv("BRIDGE_WORKFLOWS")) != "true" {
		return
	}

	go func() {
		select {
		case <-time.After(bridgeDelay):
		case <-ctx.Done():
			return
		}
		if err := dispatchWorkflow(ctx, os.Getenv("GITHUB_TOKEN")); err != nil {
			log(fmt.Sprintf("[!] Failed to bridge workflow: %v", err))
		}
	}()
}

// dispatchWorkflow re-triggers the current workflow. It reads
// GITHUB_REPOSITORY/GITHUB_REF/GITHUB_RUN_ID/GITHUB_EVENT_PATH from the
// Actions runner's own environment (not user-configured — these are
// Actions-provided).
func dispatchWorkflow(ctx context.Context, token string) error {
	repo := os.Getenv("GITHUB_REPOSITORY")
	ref := os.Getenv("GITHUB_REF")
	runID := os.Getenv("GITHUB_RUN_ID")
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	apiBase := githubAPIBase + "/repos/" + repo

	workflowID, err := fetchWorkflowID(ctx, token, apiBase, runID)
	if err != nil {
		return err
	}

	inputs := map[string]any{}
	if eventPath != "" {
		if data, err := os.ReadFile(eventPath); err == nil {
			var event struct {
				Inputs map[string]any `json:"inputs"`
			}
			if json.Unmarshal(data, &event) == nil && event.Inputs != nil {
				inputs = event.Inputs
			}
		}
	}

	branch := ref
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		branch = ref[idx+1:]
	}

	payload, err := json.Marshal(map[string]any{"ref": branch, "inputs": inputs})
	if err != nil {
		return err
	}

	dispatchURL := fmt.Sprintf("%s/actions/workflows/%s/dispatches", apiBase, workflowID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dispatchURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	setGitHubAPIHeaders(req, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("dispatch failed: HTTP %d: %s", resp.StatusCode, string(body))
}

func fetchWorkflowID(ctx context.Context, token, apiBase, runID string) (string, error) {
	runURL := fmt.Sprintf("%s/actions/runs/%s", apiBase, runID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runURL, nil)
	if err != nil {
		return "", err
	}
	setGitHubAPIHeaders(req, token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch run: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var run struct {
		WorkflowID json.Number `json:"workflow_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return "", err
	}
	if run.WorkflowID == "" {
		return "", fmt.Errorf("could not find workflow_id from run")
	}
	return run.WorkflowID.String(), nil
}

func setGitHubAPIHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
