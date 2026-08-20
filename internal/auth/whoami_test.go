package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withUserServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := UserURL
	UserURL = srv.URL
	t.Cleanup(func() { UserURL = old })
}

func TestWhoAmIReturnsVerifiedLogin(t *testing.T) {
	var gotAuth string
	withUserServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "priyam"})
	})

	login, err := WhoAmI("gho_abc123")
	if err != nil {
		t.Fatalf("WhoAmI() error = %v", err)
	}
	if login != "priyam" {
		t.Errorf("WhoAmI() = %q, want priyam", login)
	}
	if gotAuth != "Bearer gho_abc123" {
		t.Errorf("Authorization header = %q, want Bearer gho_abc123", gotAuth)
	}
}

func TestWhoAmIFailsOnInvalidToken(t *testing.T) {
	withUserServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})

	if _, err := WhoAmI("bad-token"); err == nil {
		t.Fatal("WhoAmI() error = nil, want error for an invalid token")
	}
}
