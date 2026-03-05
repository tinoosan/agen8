package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

func runRootForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func TestAuthStatus_ChatGPTLoggedOut(t *testing.T) {
	dataDir := t.TempDir()
	authProviderFlag = ""
	out, err := runRootForTest(t, "--data-dir", dataDir, "auth", "status", "--provider", authpkg.ProviderChatGPTAccount)
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Provider: chatgpt_account") {
		t.Fatalf("missing provider line:\n%s", out)
	}
	if !strings.Contains(out, "Logged in: false") {
		t.Fatalf("missing logged-out line:\n%s", out)
	}
}

func TestAuthStatus_UsesRootAuthProviderWhenProviderFlagOmitted(t *testing.T) {
	dataDir := t.TempDir()
	authProviderFlag = ""
	out, err := runRootForTest(t, "--data-dir", dataDir, "--auth-provider", authpkg.ProviderChatGPTAccount, "auth", "status")
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Provider: chatgpt_account") {
		t.Fatalf("expected root auth provider to be used:\n%s", out)
	}
}

func TestAuthStatus_ChatGPTLoggedIn(t *testing.T) {
	dataDir := t.TempDir()
	if err := authpkg.NewFileTokenStore(dataDir).Save(authpkg.OAuthTokenRecord{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		ExpiresAtUnix: time.Now().Add(time.Hour).UnixMilli(),
		AccountID:     "acct_123456",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	authProviderFlag = ""
	out, err := runRootForTest(t, "--data-dir", dataDir, "auth", "status", "--provider", authpkg.ProviderChatGPTAccount)
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Logged in: true") {
		t.Fatalf("missing logged-in line:\n%s", out)
	}
	if !strings.Contains(out, "Account: acc...456") {
		t.Fatalf("expected redacted account id:\n%s", out)
	}
}

func TestAuthLogout_ChatGPTRemovesTokenFile(t *testing.T) {
	dataDir := t.TempDir()
	store := authpkg.NewFileTokenStore(dataDir)
	if err := store.Save(authpkg.OAuthTokenRecord{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		ExpiresAtUnix: time.Now().Add(time.Hour).UnixMilli(),
		AccountID:     "acct_123456",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	authProviderFlag = ""
	out, err := runRootForTest(t, "--data-dir", dataDir, "auth", "logout", "--provider", authpkg.ProviderChatGPTAccount)
	if err != nil {
		t.Fatalf("auth logout: %v\n%s", err, out)
	}
	if _, statErr := os.Stat(store.Path()); !os.IsNotExist(statErr) {
		t.Fatalf("expected token file removed, stat err=%v", statErr)
	}
}

func TestAuthStatus_InvalidProviderFails(t *testing.T) {
	dataDir := t.TempDir()
	authProviderFlag = ""
	out, err := runRootForTest(t, "--data-dir", dataDir, "auth", "status", "--provider", "not-a-provider")
	if err == nil {
		t.Fatalf("expected error for invalid provider")
	}
	if !strings.Contains(strings.ToLower(out+err.Error()), "unsupported auth provider") {
		t.Fatalf("unexpected error: %v\n%s", err, out)
	}
}
