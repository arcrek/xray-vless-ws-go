// Package selfupdate implements the `--update`/`--rollback` OTA flow:
// checking the latest GitHub Release, downloading + verifying the matching
// platform asset against SHA256SUMS, atomically swapping it into place
// (preserving the previous binary for a single-level rollback), and
// restarting the xrayws systemd service. See the parent plan
// (plans/260903-1540-ota-self-update/plan.md) for the full design.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// githubAPIBase is overridable in tests so latestRelease can be exercised
// against an httptest.Server instead of the real GitHub API — same idiom
// as internal/ci/bridge.go's githubAPIBase.
var githubAPIBase = "https://api.github.com"

// httpClient is shared by every network call in this package (github.go,
// verify.go). A bounded per-request timeout (independent of ctx, which is
// normally only cancelled by SIGINT/SIGTERM) keeps a stalled GitHub/CDN
// connection from hanging --update/--rollback indefinitely; 5 minutes
// comfortably covers a full platform-binary download even on a slow link.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// githubRepo is this project's own repo, matching install.sh's hardcoded
// REPO and the module path in go.mod.
const githubRepo = "arcrek/xray-vless-ws-go"

// assetInfo is the subset of a GitHub Release asset this package needs.
type assetInfo struct {
	Name        string
	DownloadURL string
}

// latestRelease fetches the latest published GitHub Release for this repo
// and returns its tag plus a name->assetInfo map of its assets. Public
// repo, unauthenticated GET — same trust/rate-limit model as install.sh's
// wget calls (GitHub's unauthenticated REST limit is 60 req/hour per IP,
// nowhere near what a manually-triggered flag needs).
func latestRelease(ctx context.Context) (tag string, assets map[string]assetInfo, err error) {
	url := githubAPIBase + "/repos/" + githubRepo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", nil, fmt.Errorf("selfupdate: building release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("selfupdate: fetching latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("selfupdate: GitHub API returned HTTP %d for %s", resp.StatusCode, url)
	}

	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", nil, fmt.Errorf("selfupdate: decoding release JSON: %w", err)
	}
	if body.TagName == "" {
		return "", nil, fmt.Errorf("selfupdate: release response had no tag_name")
	}

	assets = make(map[string]assetInfo, len(body.Assets))
	for _, a := range body.Assets {
		assets[a.Name] = assetInfo{Name: a.Name, DownloadURL: a.BrowserDownloadURL}
	}
	return body.TagName, assets, nil
}
