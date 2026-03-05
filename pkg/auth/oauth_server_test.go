package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOAuthCallbackServer_Success(t *testing.T) {
	srv, err := StartOAuthCallbackServer("state_ok")
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()
	if !srv.Ready() {
		t.Fatalf("server should be ready in test environment")
	}

	resp, err := http.Get("http://" + OAuthCallbackAddr + OAuthCallbackPath + "?state=state_ok&code=abc123")
	if err != nil {
		t.Fatalf("http get callback: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	code, err := srv.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("wait code: %v", err)
	}
	if code != "abc123" {
		t.Fatalf("code=%q", code)
	}
}

func TestOAuthCallbackServer_StateMismatch(t *testing.T) {
	srv, err := StartOAuthCallbackServer("state_ok")
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()
	if !srv.Ready() {
		t.Fatalf("server should be ready in test environment")
	}

	resp, err := http.Get("http://" + OAuthCallbackAddr + OAuthCallbackPath + "?state=wrong&code=abc123")
	if err != nil {
		t.Fatalf("http get callback: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(body))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = srv.WaitForCode(ctx)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "state mismatch") {
		t.Fatalf("expected state mismatch error, got %v", err)
	}
}
