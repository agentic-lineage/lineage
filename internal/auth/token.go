package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tokenFileMode keeps the stored bearer credential readable only by its
// owner.
const tokenFileMode = 0o600

// TokenPath is where `lineage login` stores the GitHub device-flow token.
func TokenPath(home string) string {
	return filepath.Join(home, "github_token")
}

// SaveToken writes token to TokenPath(home), creating home if needed, with
// permissions that only the owner can read - this is a bearer credential
// good enough to publish as this person, handled like any other local
// secret Lineage must never leak.
func SaveToken(home, token string) error {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("create lineage home: %w", err)
	}
	path := TokenPath(home)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), tokenFileMode); err != nil {
		return fmt.Errorf("save GitHub token: %w", err)
	}
	// The mode passed to WriteFile only applies when the file is created,
	// so an existing github_token - from an older Lineage, a manual edit, a
	// restored backup - would keep whatever loose permissions it already
	// had. Re-enforce them on every save instead.
	if err := os.Chmod(path, tokenFileMode); err != nil {
		return fmt.Errorf("restrict GitHub token permissions: %w", err)
	}
	return nil
}

// LoadToken reads the token saved by SaveToken, or "" with no error if
// `lineage login` was never run - callers that also accept
// LINEAGE_PUBLISH_TOKEN treat a missing stored token as "check the env var
// instead," not as a failure.
func LoadToken(home string) (string, error) {
	data, err := os.ReadFile(TokenPath(home))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read stored GitHub token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// DeleteToken removes the token saved by SaveToken, if any. Removing a
// token that was never saved is not an error.
func DeleteToken(home string) error {
	if err := os.Remove(TokenPath(home)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stored GitHub token: %w", err)
	}
	return nil
}
