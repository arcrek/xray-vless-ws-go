package ci

import (
	"fmt"
	"os"
	"strings"
)

// ExportSecretEnv writes the ENV_CONFIG env var's content (GitHub Actions
// loads it from a repo Secret) to .env, overwriting any existing file. A
// missing ENV_CONFIG is not an error — it just means local/non-CI mode.
func ExportSecretEnv(envPath string) error {
	envConfig := os.Getenv("ENV_CONFIG")
	if envConfig == "" {
		fmt.Println("[!] ENV_CONFIG not found. Skipping .env creation (local mode).")
		return nil
	}

	fmt.Println("[*] Detected configuration from GitHub Secrets...")
	if err := os.WriteFile(envPath, []byte(strings.TrimSpace(envConfig)), 0o600); err != nil {
		return fmt.Errorf("ci: writing %s from ENV_CONFIG: %w", envPath, err)
	}
	fmt.Println("[+] Converted ENV_CONFIG to .env successfully.")
	return nil
}
