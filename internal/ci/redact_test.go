package ci

import (
	"strings"
	"testing"
)

// TestRedactCredentials guards the fix for a real finding from code review:
// runGit's error path used to embed the raw clone URL - including the
// GITHUB_TOKEN - verbatim in its error string, which flows straight into
// WatchAndUpload's LogFunc and, from there, into the (unauthenticated by
// default) embedded log server. Never let this regress.
func TestRedactCredentials(t *testing.T) {
	cases := map[string]string{
		"https://github-actions:ghp_supersecrettoken123@github.com/owner/repo.git":                                                   "https://***@github.com/owner/repo.git",
		"git clone https://github-actions:ghp_abc@github.com/owner/repo.git":                                                         "git clone https://***@github.com/owner/repo.git",
		"fatal: unable to access 'https://github-actions:ghp_abc@github.com/owner/repo.git/': The requested URL returned error: 403": "fatal: unable to access 'https://***@github.com/owner/repo.git/': The requested URL returned error: 403",
		"rev-parse HEAD": "rev-parse HEAD", // no credentials present: unchanged
	}
	for in, want := range cases {
		got := redactCredentials(in)
		if got != want {
			t.Errorf("redactCredentials(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, "ghp_supersecrettoken123") || strings.Contains(got, "ghp_abc") {
			t.Errorf("redactCredentials(%q) still contains a token-shaped credential: %q", in, got)
		}
	}
}
