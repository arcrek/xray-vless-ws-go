package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// downloadAndVerify downloads assetURL into destTmp (which must already be
// positioned in the executable's own directory — see run.go's Run and
// swap.go's atomicReplace for why), downloads sumsURL into memory, and
// verifies destTmp's sha256 against the line matching assetName in the
// SHA256SUMS body. Mirrors install.sh's download+verify step (lines
// 176-206) exactly in behavior — same two-file download, same exact-name
// match, same hard-fail-on-mismatch — just in Go. Hard-fails (and leaves
// only destTmp touched, never execPath) on any download error or a
// checksum mismatch.
func downloadAndVerify(ctx context.Context, assetURL, sumsURL, assetName, destTmp string) error {
	wantHash, err := fetchExpectedHash(ctx, sumsURL, assetName)
	if err != nil {
		return err
	}

	if err := downloadFile(ctx, assetURL, destTmp); err != nil {
		return err
	}

	gotHash, err := sha256File(destTmp)
	if err != nil {
		return fmt.Errorf("selfupdate: hashing downloaded asset: %w", err)
	}
	if gotHash != wantHash {
		return fmt.Errorf("selfupdate: checksum mismatch for %s: expected %s, got %s", assetName, wantHash, gotHash)
	}
	return nil
}

// fetchExpectedHash downloads the flat sha256sum-style SHA256SUMS body and
// returns the hex digest for the line whose filename matches assetName.
func fetchExpectedHash(ctx context.Context, sumsURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumsURL, nil)
	if err != nil {
		return "", fmt.Errorf("selfupdate: building SHA256SUMS request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("selfupdate: downloading SHA256SUMS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("selfupdate: SHA256SUMS download returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("selfupdate: reading SHA256SUMS: %w", err)
	}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// sha256sum prefixes the filename with "*" for binary-mode
		// entries on some platforms; strip it defensively.
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("selfupdate: no SHA256SUMS entry found for %s", assetName)
}

func downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("selfupdate: building asset request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("selfupdate: downloading asset: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("selfupdate: asset download returned HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("selfupdate: opening temp file %s: %w", destPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("selfupdate: writing downloaded asset: %w", err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
