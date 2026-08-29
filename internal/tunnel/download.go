package tunnel

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// CloudflaredVersion is pinned the same way xray-core is pinned — floating
// on the "latest release" API without a fallback risks a silent asset-name
// break (see Phase 3 risk note); verified against the cloudflare/cloudflared
// GitHub releases page at implementation time.
const CloudflaredVersion = "2026.3.0"

const baseDownloadURL = "https://github.com/cloudflare/cloudflared/releases/download/" + CloudflaredVersion

// assetInfo names the release asset for the current OS/arch and the local
// binary filename it should be installed as, ported from
// download-cloudflared.py's get_download_info().
type assetInfo struct {
	remoteAsset string
	localName   string
	isArchive   bool // true only for the macOS .tgz asset
}

func currentAsset() (assetInfo, error) {
	sys := runtime.GOOS
	arch := runtime.GOARCH

	switch sys {
	case "windows":
		if arch == "amd64" {
			return assetInfo{"cloudflared-windows-amd64.exe", "cloudflared.exe", false}, nil
		}
		return assetInfo{"cloudflared-windows-386.exe", "cloudflared.exe", false}, nil

	case "linux":
		switch arch {
		case "arm64":
			return assetInfo{"cloudflared-linux-arm64", "cloudflared", false}, nil
		case "arm":
			return assetInfo{"cloudflared-linux-arm", "cloudflared", false}, nil
		case "amd64":
			return assetInfo{"cloudflared-linux-amd64", "cloudflared", false}, nil
		default:
			return assetInfo{"cloudflared-linux-386", "cloudflared", false}, nil
		}

	case "darwin":
		if arch == "arm64" {
			return assetInfo{"cloudflared-darwin-arm64.tgz", "cloudflared", true}, nil
		}
		return assetInfo{"cloudflared-darwin-amd64.tgz", "cloudflared", true}, nil

	case "android":
		return assetInfo{}, fmt.Errorf("tunnel: cloudflared cannot run directly on Android/Termux; install via 'pkg install cloudflared' or an Ubuntu proot distro")
	}

	return assetInfo{}, fmt.Errorf("tunnel: unsupported OS/arch %s/%s", sys, arch)
}

// EnsureBinary returns the local cloudflared binary path, downloading and
// installing it if not already present in the working directory. Ported
// from download-cloudflared.py's install_cloudflared().
//
// The returned path is always absolute. This matters: Launch() passes it
// straight to exec.Command(binPath, ...), and os/exec only resolves a name
// relative to the *current process's* $PATH when the string has no path
// separator — filepath.Join(".", "cloudflared") collapses to the bare
// string "cloudflared" (no separator), which os/exec then looks up on
// $PATH instead of finding the binary that was just downloaded into dir.
// A caller passing dir="." (as cmd/xrayws/main.go does) would otherwise
// silently fail to ever launch cloudflared unless a system-wide
// "cloudflared" happened to already be on $PATH — found by an independent
// test pass, confirmed by direct repro, not a hypothetical.
func EnsureBinary(ctx context.Context, dir string) (string, error) {
	asset, err := currentAsset()
	if err != nil {
		return "", err
	}

	localPath := filepath.Join(dir, asset.localName)
	if _, statErr := os.Stat(localPath); statErr == nil {
		// Found by testing this fix: an interrupted prior download (process
		// killed mid-write, disk full, etc.) can leave a present-but-partial
		// or non-executable file here; the old code trusted existence alone
		// and skipped straight past the chmod that only ran on the
		// fresh-download branch. Re-asserting the executable bit on the
		// fast path is a cheap no-op when the file is already fine, and
		// closes that gap when it isn't. It does NOT re-validate the
		// file's *content* (no checksum available for this endpoint) — a
		// truncated-but-executable-bit-set binary would still slip through
		// and fail at exec time with a clearer error than silently hanging.
		if err := ensureExecutable(localPath); err != nil {
			return "", err
		}
		return absPath(localPath)
	}

	url := fmt.Sprintf("%s/%s", baseDownloadURL, asset.remoteAsset)
	fmt.Printf("[*] Downloading cloudflared: %s\n", url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("tunnel: building download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tunnel: downloading cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tunnel: downloading cloudflared: HTTP %d for %s", resp.StatusCode, url)
	}

	if asset.isArchive {
		if err := extractTarGzBinary(resp.Body, asset.localName, localPath); err != nil {
			return "", fmt.Errorf("tunnel: extracting cloudflared archive: %w", err)
		}
	} else {
		f, err := os.Create(localPath)
		if err != nil {
			return "", fmt.Errorf("tunnel: creating %s: %w", localPath, err)
		}
		_, copyErr := io.Copy(f, resp.Body)
		closeErr := f.Close()
		if copyErr != nil {
			return "", fmt.Errorf("tunnel: writing %s: %w", localPath, copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("tunnel: closing %s: %w", localPath, closeErr)
		}
	}

	if err := ensureExecutable(localPath); err != nil {
		return "", err
	}

	fmt.Printf("[+] Installed cloudflared at %s\n", localPath)
	return absPath(localPath)
}

func ensureExecutable(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("tunnel: chmod %s: %w", path, err)
	}
	return nil
}

// absPath resolves p to an absolute path, wrapped so both EnsureBinary
// return points funnel through the same error handling.
func absPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("tunnel: resolving absolute path for %s: %w", p, err)
	}
	return abs, nil
}

// extractTarGzBinary pulls the named binary out of a .tar.gz stream (the
// macOS release asset) and writes it to destPath.
func extractTarGzBinary(r io.Reader, binaryName, destPath string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("binary %q not found in archive", binaryName)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		out, err := os.Create(destPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
}
