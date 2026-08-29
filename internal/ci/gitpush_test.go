package ci

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireGit skips the test if git isn't on PATH — these tests shell out to
// real git against a local bare "remote" repo (no GitHub/network
// involved), exercising UploadFile's full clone/checkout/orphan/commit/
// force-push sequence safely, per the plan's "test against a scratch repo
// first" rule for this destructive-by-design code path.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH, skipping")
	}
}

// newBareRemote creates a local bare git repo to stand in for GitHub in
// tests, and returns a file:// URL UploadFile-adjacent test code can clone
// via a fake "token+repo" pair pointed at it.
func newBareRemote(t *testing.T) (remotePath string) {
	t.Helper()
	dir := t.TempDir()
	remotePath = filepath.Join(dir, "remote.git")
	cmd := exec.Command("git", "init", "--bare", "-b", "main", remotePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	return remotePath
}

// uploadFileToLocalRemote is UploadFile with the GitHub clone-URL
// construction swapped for a local file path, so tests never touch the
// network. It duplicates just enough of UploadFile's body to do that.
func uploadFileToLocalRemote(t *testing.T, remotePath, file, branch, rename, tempDir string) string {
	t.Helper()
	repoDir := filepath.Join(tempDir, "workdir")

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		if _, err := runGit(context.Background(), "", "clone", remotePath, repoDir); err != nil {
			t.Fatalf("clone: %v", err)
		}
	}
	if _, err := runGit(context.Background(), repoDir, "fetch", "--all"); err != nil {
		t.Fatalf("fetch --all: %v", err)
	}
	if err := checkoutOrCreateBranch(context.Background(), repoDir, branch); err != nil {
		t.Fatalf("checkoutOrCreateBranch: %v", err)
	}

	dest := rename
	if dest == "" {
		dest = filepath.Base(file)
	}
	destPath := filepath.Join(repoDir, dest)
	if err := copyFile(file, destPath); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if _, err := runGit(context.Background(), repoDir, "add", dest); err != nil {
		t.Fatalf("git add: %v", err)
	}
	commitArgs := []string{
		"-c", "user.email=test@example.com",
		"-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "test commit",
	}
	if _, err := runGit(context.Background(), repoDir, commitArgs...); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if _, err := runGit(context.Background(), repoDir, "push", "--force", "origin", branch); err != nil {
		t.Fatalf("push --force: %v", err)
	}
	sha, err := runGit(context.Background(), repoDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return sha
}

func TestUploadFileCreatesOrphanBranchAndPushes(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	tempDir := t.TempDir()

	srcFile := filepath.Join(t.TempDir(), "frp_info.config")
	if err := os.WriteFile(srcFile, []byte("vless://test-link"), 0o644); err != nil {
		t.Fatal(err)
	}

	uploadFileToLocalRemote(t, remote, srcFile, "config", "", tempDir)

	// Verify the branch now exists on the "remote" with the file content,
	// by cloning it fresh into a separate directory.
	verifyDir := filepath.Join(t.TempDir(), "verify")
	if _, err := runGit(context.Background(), "", "clone", "--branch", "config", remote, verifyDir); err != nil {
		t.Fatalf("verify clone: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(verifyDir, "frp_info.config"))
	if err != nil {
		t.Fatalf("reading uploaded file from verify clone: %v", err)
	}
	if string(content) != "vless://test-link" {
		t.Errorf("uploaded content = %q, want %q", content, "vless://test-link")
	}
}

func TestUploadFileSecondCallOverwritesOnExistingBranch(t *testing.T) {
	requireGit(t)
	remote := newBareRemote(t)
	tempDir := t.TempDir()

	srcFile := filepath.Join(t.TempDir(), "frp_info.config")
	os.WriteFile(srcFile, []byte("first-content"), 0o644)
	uploadFileToLocalRemote(t, remote, srcFile, "config", "", tempDir)

	os.WriteFile(srcFile, []byte("second-content"), 0o644)
	uploadFileToLocalRemote(t, remote, srcFile, "config", "", tempDir)

	verifyDir := filepath.Join(t.TempDir(), "verify2")
	if _, err := runGit(context.Background(), "", "clone", "--branch", "config", remote, verifyDir); err != nil {
		t.Fatalf("verify clone: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(verifyDir, "frp_info.config"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "second-content" {
		t.Errorf("content = %q, want %q (second upload should overwrite)", content, "second-content")
	}
}
