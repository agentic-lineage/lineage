package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lineage-dev/lineage/internal/auth"
	"github.com/lineage-dev/lineage/internal/config"
)

// withGitHubAuthServer points the auth package's device-flow and identity
// endpoints at a local stub for the duration of the test, restoring the
// real GitHub URLs afterward.
func withGitHubAuthServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	oldDC, oldAT, oldUser := auth.DeviceCodeURL, auth.AccessTokenURL, auth.UserURL
	auth.DeviceCodeURL = srv.URL + "/login/device/code"
	auth.AccessTokenURL = srv.URL + "/login/oauth/access_token"
	auth.UserURL = srv.URL + "/user"
	t.Cleanup(func() {
		auth.DeviceCodeURL, auth.AccessTokenURL, auth.UserURL = oldDC, oldAT, oldUser
	})
}

func TestLoginStoresTokenAndPrintsLogin(t *testing.T) {
	withGitHubAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device/code"):
			_ = json.NewEncoder(w).Encode(auth.DeviceCode{
				DeviceCode: "dc-1", UserCode: "ABCD-1234",
				VerificationURI: "https://github.com/login/device", ExpiresIn: 30, Interval: 1,
			})
		case strings.HasSuffix(r.URL.Path, "/oauth/access_token"):
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_test123"})
		case strings.HasSuffix(r.URL.Path, "/user"):
			_ = json.NewEncoder(w).Encode(map[string]string{"login": "priyam"})
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
		}
	})

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv(config.HomeEnv, home)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"login"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("login error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "logged in as priyam") {
		t.Fatalf("stdout = %q, want confirmation naming the logged-in user", stdout.String())
	}

	got, err := auth.LoadToken(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gho_test123" {
		t.Fatalf("LoadToken() after login = %q, want gho_test123", got)
	}
}

func TestWhoAmIReportsNotLoggedIn(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv(config.HomeEnv, home)
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "")

	var stdout, stderr bytes.Buffer
	err := Execute(nil, []string{"whoami"}, nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("whoami with nothing logged in: error = nil, want error")
	}
	if !strings.Contains(stderr.String(), "not logged in") {
		t.Fatalf("stderr = %q, want a message pointing at `lineage login`", stderr.String())
	}
}

func TestWhoAmIAfterLoginRoundTrip(t *testing.T) {
	withGitHubAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "priyam"})
	})

	home := filepath.Join(t.TempDir(), "home")
	if err := auth.SaveToken(home, "gho_test123"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.HomeEnv, home)
	t.Setenv("LINEAGE_PUBLISH_TOKEN", "")

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"whoami"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("whoami error = %v stderr=%s", err, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "priyam" {
		t.Fatalf("stdout = %q, want priyam", stdout.String())
	}
}

func TestLogoutRemovesStoredToken(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := auth.SaveToken(home, "gho_test123"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.HomeEnv, home)

	var stdout, stderr bytes.Buffer
	if err := Execute(nil, []string{"logout"}, nil, &stdout, &stderr); err != nil {
		t.Fatalf("logout error = %v stderr=%s", err, stderr.String())
	}

	got, err := auth.LoadToken(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("LoadToken() after logout = %q, want empty", got)
	}
}
