// Package auth implements Lineage's publisher identity: GitHub's OAuth
// Device Flow, the same mechanism `gh auth login` uses. Publisher identity
// is a GitHub-verified login rather than a token the site operator has to
// mint and distribute by hand - see docs/decisions/
// 0012-v1-distribution-contract-and-receiver-activation.md, decision 6b.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ClientID is Lineage's public GitHub OAuth App client id (org:
// agentic-lineage, Device Flow enabled). It is not a secret: GitHub's
// device flow for a public/native client never requires a client secret to
// exchange a device code for an access token.
const ClientID = "Ov23liHPdq5xNJy2DU8W"

// scope requests just enough to identify who's publishing. Lineage never
// needs write access to a publisher's own account or repos - the registry
// repo is written to by the website's own token, not the publisher's.
const scope = "read:user"

// Overridable for tests; point at github.com in production.
var (
	DeviceCodeURL  = "https://github.com/login/device/code"
	AccessTokenURL = "https://github.com/login/oauth/access_token"
)

// DeviceCode is what a receiver sees and approves in a browser.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// RequestDeviceCode starts GitHub's device flow.
func RequestDeviceCode() (DeviceCode, error) {
	body, err := postForm(DeviceCodeURL, url.Values{"client_id": {ClientID}, "scope": {scope}})
	if err != nil {
		return DeviceCode{}, fmt.Errorf("request device code: %w", err)
	}
	var dc DeviceCode
	if err := json.Unmarshal(body, &dc); err != nil {
		return DeviceCode{}, fmt.Errorf("parse device code response: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return DeviceCode{}, fmt.Errorf("device code response missing device_code/user_code: %s", string(body))
	}
	return dc, nil
}

var (
	errAuthorizationPending = errors.New("authorization pending")
	errSlowDown             = errors.New("slow down")
	// ErrExpired is returned by PollForToken when the device code expires
	// before the user approves it - the caller should start over with a
	// fresh RequestDeviceCode.
	ErrExpired = errors.New("device code expired before it was approved; run `lineage login` again")
	// ErrDenied is returned by PollForToken when the user explicitly
	// declines the authorization request.
	ErrDenied = errors.New("authorization was denied")
)

// PollForToken polls GitHub's token endpoint, following dc's own interval
// (backing off further whenever GitHub asks to slow_down), until the user
// approves, the device code expires, or ctx is cancelled.
func PollForToken(ctx context.Context, dc DeviceCode) (string, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		if time.Now().After(deadline) {
			return "", ErrExpired
		}

		token, err := pollOnce(dc.DeviceCode)
		switch {
		case err == nil:
			return token, nil
		case errors.Is(err, errAuthorizationPending):
			continue
		case errors.Is(err, errSlowDown):
			interval += 5 * time.Second
			continue
		default:
			return "", err
		}
	}
}

func pollOnce(deviceCode string) (string, error) {
	body, err := postForm(AccessTokenURL, url.Values{
		"client_id":   {ClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	})
	if err != nil {
		return "", fmt.Errorf("poll for token: %w", err)
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	switch parsed.Error {
	case "":
		if parsed.AccessToken == "" {
			return "", fmt.Errorf("token response had neither an access token nor an error: %s", string(body))
		}
		return parsed.AccessToken, nil
	case "authorization_pending":
		return "", errAuthorizationPending
	case "slow_down":
		return "", errSlowDown
	case "expired_token":
		return "", ErrExpired
	case "access_denied":
		return "", ErrDenied
	default:
		return "", fmt.Errorf("device flow error: %s", parsed.Error)
	}
}

func postForm(rawURL string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
