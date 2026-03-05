package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

func TestEnsureRuntimeAuthReady_APIKeyProviderSkipsOAuthCheck(t *testing.T) {
	if err := EnsureRuntimeAuthReady(context.Background(), t.TempDir(), authpkg.ProviderAPIKey); err != nil {
		t.Fatalf("EnsureRuntimeAuthReady(api_key): %v", err)
	}
}

func TestEnsureRuntimeAuthReady_ChatGPTProviderRequiresLogin(t *testing.T) {
	err := EnsureRuntimeAuthReady(context.Background(), t.TempDir(), authpkg.ProviderChatGPTAccount)
	if err == nil {
		t.Fatalf("expected login-required error")
	}
	if !errors.Is(err, ErrChatGPTAccountLoginRequired) {
		t.Fatalf("expected ErrChatGPTAccountLoginRequired, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "auth login") {
		t.Fatalf("expected relogin guidance, got: %v", err)
	}
}

func TestEnsureRuntimeAuthReady_ChatGPTProviderAcceptsStoredToken(t *testing.T) {
	origClient := chatGPTCodexReadinessHTTPClient
	origURL := chatGPTCodexReadinessURL
	t.Cleanup(func() {
		chatGPTCodexReadinessHTTPClient = origClient
		chatGPTCodexReadinessURL = origURL
	})
	chatGPTCodexReadinessURL = "https://chatgpt.com/backend-api/codex/responses"
	chatGPTCodexReadinessHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusMethodNotAllowed,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"detail":"Method Not Allowed"}`)),
				Request:    req,
			}, nil
		}),
	}

	dataDir := t.TempDir()
	store := authpkg.NewFileTokenStore(dataDir)
	if err := store.Save(authpkg.OAuthTokenRecord{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		AccountID:     "acct_123",
		ExpiresAtUnix: time.Now().UTC().Add(1 * time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	if err := EnsureRuntimeAuthReady(context.Background(), dataDir, authpkg.ProviderChatGPTAccount); err != nil {
		t.Fatalf("EnsureRuntimeAuthReady(chatgpt_account): %v", err)
	}
}

func TestEnsureRuntimeAuthReady_ChatGPTProviderProbeUnauthorized(t *testing.T) {
	origClient := chatGPTCodexReadinessHTTPClient
	origURL := chatGPTCodexReadinessURL
	t.Cleanup(func() {
		chatGPTCodexReadinessHTTPClient = origClient
		chatGPTCodexReadinessURL = origURL
	})
	chatGPTCodexReadinessURL = "https://chatgpt.com/backend-api/codex/responses"
	chatGPTCodexReadinessHTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"detail":"unauthorized"}`)),
				Request:    req,
			}, nil
		}),
	}

	dataDir := t.TempDir()
	store := authpkg.NewFileTokenStore(dataDir)
	if err := store.Save(authpkg.OAuthTokenRecord{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		AccountID:     "acct_123",
		ExpiresAtUnix: time.Now().UTC().Add(1 * time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	err := EnsureRuntimeAuthReady(context.Background(), dataDir, authpkg.ProviderChatGPTAccount)
	if err == nil {
		t.Fatalf("expected readiness probe error")
	}
	if !errors.Is(err, ErrChatGPTAccountLoginRequired) {
		t.Fatalf("expected ErrChatGPTAccountLoginRequired, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
