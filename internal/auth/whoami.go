package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Overridable for tests; points at api.github.com in production.
var UserURL = "https://api.github.com/user"

// WhoAmI resolves the GitHub login a token identifies, the same way the
// website verifies a publish request (landing/api/_lib/auth.js) - a token
// is never trusted to say who it belongs to, only GitHub is.
func WhoAmI(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, UserURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("verify GitHub identity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verify GitHub identity failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse GitHub identity response: %w", err)
	}
	if parsed.Login == "" {
		return "", fmt.Errorf("GitHub identity response had no login: %s", string(body))
	}
	return parsed.Login, nil
}
