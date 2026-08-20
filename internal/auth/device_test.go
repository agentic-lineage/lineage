package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func withDeviceCodeServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldDC, oldAT := DeviceCodeURL, AccessTokenURL
	DeviceCodeURL = srv.URL + "/device/code"
	AccessTokenURL = srv.URL + "/oauth/access_token"
	t.Cleanup(func() { DeviceCodeURL, AccessTokenURL = oldDC, oldAT })
}

func TestRequestDeviceCode(t *testing.T) {
	withDeviceCodeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/device/code" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("scope") != "read:user" {
			t.Fatalf("scope = %q, want read:user", r.Form.Get("scope"))
		}
		_ = json.NewEncoder(w).Encode(DeviceCode{
			DeviceCode: "dc-1", UserCode: "ABCD-1234",
			VerificationURI: "https://github.com/login/device", ExpiresIn: 900, Interval: 1,
		})
	})

	dc, err := RequestDeviceCode()
	if err != nil {
		t.Fatalf("RequestDeviceCode() error = %v", err)
	}
	if dc.UserCode != "ABCD-1234" || dc.DeviceCode != "dc-1" {
		t.Errorf("RequestDeviceCode() = %+v, want user_code=ABCD-1234 device_code=dc-1", dc)
	}
}

func TestPollForTokenSucceedsAfterPending(t *testing.T) {
	var calls int32
	withDeviceCodeServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_test123"})
	})

	// Interval is in whole seconds (matches GitHub's real response shape),
	// so this test takes a few real seconds rather than mocking time.
	dc := DeviceCode{DeviceCode: "dc-1", ExpiresIn: 30, Interval: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := PollForToken(ctx, dc)
	if err != nil {
		t.Fatalf("PollForToken() error = %v", err)
	}
	if token != "gho_test123" {
		t.Errorf("PollForToken() = %q, want gho_test123", token)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 poll calls, got %d", calls)
	}
}

func TestPollForTokenReturnsErrDenied(t *testing.T) {
	withDeviceCodeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	})

	dc := DeviceCode{DeviceCode: "dc-1", ExpiresIn: 30, Interval: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := PollForToken(ctx, dc)
	if err != ErrDenied {
		t.Fatalf("PollForToken() error = %v, want ErrDenied", err)
	}
}

func TestPollForTokenExpiresWithoutApproval(t *testing.T) {
	withDeviceCodeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	})

	// ExpiresIn shorter than a single poll interval means the deadline check
	// after the first wait should already report expired.
	dc := DeviceCode{DeviceCode: "dc-1", ExpiresIn: 1, Interval: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := PollForToken(ctx, dc)
	if err != ErrExpired {
		t.Fatalf("PollForToken() error = %v, want ErrExpired", err)
	}
}

func TestPollForTokenRespectsContextCancellation(t *testing.T) {
	withDeviceCodeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	})

	dc := DeviceCode{DeviceCode: "dc-1", ExpiresIn: 30, Interval: 1}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	_, err := PollForToken(ctx, dc)
	if err != context.Canceled {
		t.Fatalf("PollForToken() error = %v, want context.Canceled", err)
	}
}
