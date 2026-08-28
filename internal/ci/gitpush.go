package ci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// UploadFile clones-or-reuses a local working copy of repo, checks out (or
// creates, as an orphan) branch, copies file into it (as rename, or its
// basename if rename is empty), commits, and force-pushes. Ported from
// github_utils.py:5-87 (upload_file()), shelling out to the `git` CLI
// rather than using go-git — see plan decision log #11 for the tradeoff
// (this makes the binary depend on `git` being on PATH, accepted since the
// GitHub Actions runner already has it and go-git's force-push/orphan-branch
// parity carries its own edge-case risk).
//
// The hidden/-prefix branch auto-delete-after-push behavior
// (github_utils.py:85-87) is intentionally NOT ported — confirmed dead code
// in this repo (no caller ever uses a "hidden/"-prefixed branch), dropped
// per plan decision log #12 (YAGNI, contradicts "don't over-build").
//
// Security note: like the Python version, the token is embedded in the
// clone URL, which is visible in `ps aux` on multi-user systems — a
// pre-existing limitation carried over deliberately, not a new regression.
func UploadFile(ctx context.Context, token, repo, file, branch, rename, tempDir string) (sha string, err error) {
	repoDir := filepath.Join(tempDir, repo)

	if _, statErr := os.Stat(repoDir); os.IsNotExist(statErr) {
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return "", fmt.Errorf("ci: creating %s: %w", filepath.Dir(repoDir), err)
		}
		cloneURL := fmt.Sprintf("https://github-actions:%s@github.com/%s.git", token, repo)
		if _, err := runGit(ctx, "", "clone", cloneURL, repoDir); err != nil {
			return "", fmt.Errorf("ci: cloning %s: %w", repo, err)
		}
	}

	if _, err := runGit(ctx, repoDir, "fetch", "--all"); err != nil {
		return "", fmt.Errorf("ci: fetch --all: %w", err)
	}

	if err := checkoutOrCreateBranch(ctx, repoDir, branch); err != nil {
		return "", err
	}

	dest := rename
	if dest == "" {
		dest = filepath.Base(file)
	} else {
		dest = strings.TrimPrefix(dest, "/")
	}
	destPath := filepath.Join(repoDir, dest)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("ci: creating destination dir for %s: %w", dest, err)
	}
	if err := copyFile(file, destPath); err != nil {
		return "", fmt.Errorf("ci: copying %s to %s: %w", file, destPath, err)
	}

	if _, err := runGit(ctx, repoDir, "add", dest); err != nil {
		return "", fmt.Errorf("ci: git add %s: %w", dest, err)
	}

	commitArgs := []string{
		"-c", "user.email=xray-vless-ws-go-ci@users.noreply.github.com",
		"-c", "user.name=xray-vless-ws-go CI",
		"commit", "--allow-empty", "-m", fmt.Sprintf("Add %s to %s branch", file, branch),
	}
	if _, err := runGit(ctx, repoDir, commitArgs...); err != nil {
		return "", fmt.Errorf("ci: git commit: %w", err)
	}

	if _, err := runGit(ctx, repoDir, "push", "--force", "origin", branch); err != nil {
		return "", fmt.Errorf("ci: force-pushing %s: %w", branch, err)
	}

	out, err := runGit(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("ci: rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// checkoutOrCreateBranch ports the checkout-or-create-orphan dance from
// github_utils.py:29-53: try a local checkout, then a remote-tracking
// checkout, then fall back to creating a fresh orphan branch with an empty
// initial commit (pushed immediately so the branch exists on the remote).
func checkoutOrCreateBranch(ctx context.Context, repoDir, branch string) error {
	if _, err := runGit(ctx, repoDir, "checkout", branch); err == nil {
		return nil
	}

	if _, err := runGit(ctx, repoDir, "checkout", "-b", branch, "origin/"+branch); err == nil {
		return nil
	}

	if _, err := runGit(ctx, repoDir, "checkout", "--orphan", branch); err != nil {
		return fmt.Errorf("ci: creating orphan branch %s: %w", branch, err)
	}
	if _, err := runGit(ctx, repoDir, "reset", "--hard"); err != nil {
		return fmt.Errorf("ci: reset --hard on orphan branch: %w", err)
	}
	commitArgs := []string{
		"-c", "user.email=xray-vless-ws-go-ci@users.noreply.github.com",
		"-c", "user.name=xray-vless-ws-go CI",
		"commit", "--allow-empty", "-m", "Initial empty commit on orphan branch",
	}
	if _, err := runGit(ctx, repoDir, commitArgs...); err != nil {
		return fmt.Errorf("ci: initial orphan commit: %w", err)
	}
	if _, err := runGit(ctx, repoDir, "push", "origin", branch); err != nil {
		return fmt.Errorf("ci: pushing new orphan branch %s: %w", branch, err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// credentialURLPattern matches the userinfo portion of a URL
// (scheme://user:token@host/...), e.g. the GITHUB_TOKEN embedded in the
// clone URL this package builds. Used to redact it before the token can
// ever reach an error string, a log line, or the embedded log server.
var credentialURLPattern = regexp.MustCompile(`://[^/@\s]+@`)

func redactCredentials(s string) string {
	return credentialURLPattern.ReplaceAllString(s, "://***@")
}

// runGit runs `git <args...>` in dir (repoDir, or "" for none, e.g. the
// initial clone) and returns combined stdout+stderr.
//
// Both the argv (which, for the initial clone, embeds the GITHUB_TOKEN in
// the clone URL — see UploadFile) and git's own stdout/stderr (git's own
// "fatal: unable to access '<url-with-credentials>'" error text embeds the
// same URL) are redacted before being placed in the returned error or
// output. Without this, a transient clone failure would put the live
// token into this package's caller chain — including, ultimately,
// WatchAndUpload's LogFunc, which main.go feeds straight into the
// unauthenticated-by-default embedded log server (see Push in
// internal/logserver). Never skip this redaction to "just see the error
// output" while debugging; use `git` directly by hand instead.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	safeOut := redactCredentials(string(out))
	if err != nil {
		safeArgs := redactCredentials(strings.Join(args, " "))
		return safeOut, fmt.Errorf("git %s: %w: %s", safeArgs, err, strings.TrimSpace(safeOut))
	}
	return safeOut, nil
}
