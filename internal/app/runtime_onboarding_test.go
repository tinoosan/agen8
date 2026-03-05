package app

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

func TestEnsureRuntimeCredentials_UsesKeychainFallback(t *testing.T) {
	t.Setenv(openRouterAPIKeyEnv, "")
	origGet := keyringGet
	t.Cleanup(func() { keyringGet = origGet })
	keyringGet = func(service, user string) (string, error) {
		return "key-from-keychain", nil
	}
	if err := ensureRuntimeCredentials(t.TempDir(), false, nil, nil); err != nil {
		t.Fatalf("ensureRuntimeCredentials: %v", err)
	}
	if got := os.Getenv(openRouterAPIKeyEnv); got != "key-from-keychain" {
		t.Fatalf("api key=%q", got)
	}
}

func TestEnsureRuntimeCredentials_HeadlessMissingKeyFails(t *testing.T) {
	t.Setenv(openRouterAPIKeyEnv, "")
	origGet := keyringGet
	t.Cleanup(func() { keyringGet = origGet })
	keyringGet = func(service, user string) (string, error) {
		return "", nil
	}
	dataDir := t.TempDir()
	err := ensureRuntimeCredentials(dataDir, false, nil, nil)
	if err == nil {
		t.Fatalf("expected missing key error")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "openrouter_api_key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(msg, strings.ToLower(filepath.Join(dataDir, "config.toml"))) {
		t.Fatalf("expected config path in error: %v", err)
	}
}

func TestRunInteractiveRuntimeOnboarding_PersistsKeyAndModel(t *testing.T) {
	t.Setenv(openRouterAPIKeyEnv, "")
	t.Setenv(openRouterModelEnv, "")
	dataDir := t.TempDir()
	t.Chdir(t.TempDir())

	origGet := keyringGet
	origSet := keyringSet
	origPrompt := onboardingPrompt
	origSecret := onboardingSecret
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
		onboardingPrompt = origPrompt
		onboardingSecret = origSecret
	})
	keyringGet = func(service, user string) (string, error) { return "", nil }
	var storedService, storedAccount, storedKey string
	keyringSet = func(service, user, password string) error {
		storedService = service
		storedAccount = user
		storedKey = password
		return nil
	}
	onboardingPrompt = func(reader *bufio.Reader, out io.Writer, label, def string) (string, error) {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "provider":
			return "openrouter", nil
		case "default model":
			return "z-ai/GLM-5", nil
		default:
			return def, nil
		}
	}
	onboardingSecret = func(in *os.File, out io.Writer, label string) (string, error) {
		return "interactive-key", nil
	}

	var out bytes.Buffer
	if err := runInteractiveRuntimeOnboarding(dataDir, nil, &out); err != nil {
		t.Fatalf("runInteractiveRuntimeOnboarding: %v", err)
	}
	output := out.String()
	for _, want := range []string{"Welcome to Agen8", "Setup complete", "openrouter", "z-ai/GLM-5", "agen8 daemon"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
	if storedService != agen8KeyringService || storedAccount != "openrouter.api_key" || storedKey != "interactive-key" {
		t.Fatalf("unexpected keyring write: %q %q %q", storedService, storedAccount, storedKey)
	}
	if got := os.Getenv(openRouterAPIKeyEnv); got != "interactive-key" {
		t.Fatalf("api key=%q", got)
	}
	cfgRaw, err := os.ReadFile(filepath.Join(dataDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if !strings.Contains(string(cfgRaw), `model = "z-ai/GLM-5"`) {
		t.Fatalf("expected model persisted, got:\n%s", string(cfgRaw))
	}
}

func TestPrintOnboardingSummary(t *testing.T) {
	var buf bytes.Buffer
	printOnboardingSummary(&buf, "openrouter", "z-ai/GLM-5", "saved to OS keychain", "/tmp/agen8-test", true, "/knowledge")
	output := buf.String()
	for _, want := range []string{
		"Setup complete",
		"Provider:   openrouter",
		"Model:      z-ai/GLM-5",
		"Auth:       saved to OS keychain",
		"Obsidian:   detected",
		"Vault:      /knowledge",
		"/tmp/agen8-test/config.toml",
		"agen8 daemon",
		"agen8 monitor",
		"Next steps",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
}

type fakeChatGPTProvider struct {
	loginCalled bool
	status      authpkg.Status
	loginErr    error
}

func (f *fakeChatGPTProvider) Name() string { return authpkg.ProviderChatGPTAccount }
func (f *fakeChatGPTProvider) Login(ctx context.Context, interactive bool) error {
	_ = ctx
	_ = interactive
	f.loginCalled = true
	return f.loginErr
}
func (f *fakeChatGPTProvider) Status(ctx context.Context) (authpkg.Status, error) {
	_ = ctx
	return f.status, nil
}
func (f *fakeChatGPTProvider) Logout(ctx context.Context) error {
	_ = ctx
	return nil
}
func (f *fakeChatGPTProvider) AccessToken(ctx context.Context) (authpkg.Token, error) {
	_ = ctx
	return authpkg.Token{}, nil
}

func TestEnsureRuntimeCredentials_ChatGPTHeadlessMissingTokenFailsWithHint(t *testing.T) {
	t.Setenv(authpkg.EnvAuthProvider, authpkg.ProviderChatGPTAccount)
	dataDir := t.TempDir()
	err := ensureRuntimeCredentials(dataDir, false, nil, nil)
	if err == nil {
		t.Fatalf("expected missing oauth login error")
	}
	if !strings.Contains(err.Error(), "agen8 auth login --provider chatgpt_account") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureRuntimeCredentials_ChatGPTWithStoredTokenPasses(t *testing.T) {
	t.Setenv(authpkg.EnvAuthProvider, authpkg.ProviderChatGPTAccount)
	dataDir := t.TempDir()
	store := authpkg.NewFileTokenStore(dataDir)
	if err := store.Save(authpkg.OAuthTokenRecord{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		ExpiresAtUnix: time.Now().Add(time.Hour).UnixMilli(),
		AccountID:     "acct_ok",
	}); err != nil {
		t.Fatalf("seed token store: %v", err)
	}
	if err := ensureRuntimeCredentials(dataDir, false, nil, nil); err != nil {
		t.Fatalf("ensureRuntimeCredentials: %v", err)
	}
}

func TestRunInteractiveRuntimeOnboarding_ChatGPTProviderFlow(t *testing.T) {
	t.Setenv(openRouterAPIKeyEnv, "")
	_ = os.Unsetenv(openRouterModelEnv)
	t.Setenv(authpkg.EnvAuthProvider, "")
	dataDir := t.TempDir()
	t.Chdir(t.TempDir())

	origPrompt := onboardingPrompt
	origNewAuth := newAuthProvider
	origSecret := onboardingSecret
	origSet := keyringSet
	t.Cleanup(func() {
		onboardingPrompt = origPrompt
		newAuthProvider = origNewAuth
		onboardingSecret = origSecret
		keyringSet = origSet
	})
	onboardingPrompt = func(reader *bufio.Reader, out io.Writer, label, def string) (string, error) {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "provider":
			return authpkg.ProviderChatGPTAccount, nil
		case "default model":
			return "openai/gpt-5", nil
		default:
			return def, nil
		}
	}
	// Should never be called for chatgpt_account onboarding.
	onboardingSecret = func(in *os.File, out io.Writer, label string) (string, error) {
		t.Fatalf("unexpected secret prompt for chatgpt_account")
		return "", nil
	}
	keyringSet = func(service, user, password string) error {
		t.Fatalf("unexpected keyring write for chatgpt_account")
		return nil
	}
	fake := &fakeChatGPTProvider{
		status: authpkg.Status{Provider: authpkg.ProviderChatGPTAccount, LoggedIn: true},
	}
	newAuthProvider = func(opts authpkg.ChatGPTAccountProviderOptions) (authpkg.Provider, error) {
		return fake, nil
	}

	var out bytes.Buffer
	if err := runInteractiveRuntimeOnboarding(dataDir, nil, &out); err != nil {
		t.Fatalf("runInteractiveRuntimeOnboarding: %v", err)
	}
	if !fake.loginCalled {
		t.Fatalf("expected oauth login to be invoked")
	}
	if got := os.Getenv(authpkg.EnvAuthProvider); got != authpkg.ProviderChatGPTAccount {
		t.Fatalf("auth provider env=%q", got)
	}
	if got := os.Getenv(openRouterModelEnv); got != "openai/gpt-5" {
		t.Fatalf("model env=%q", got)
	}
	text := out.String()
	if !strings.Contains(text, "Provider:   chatgpt_account") {
		t.Fatalf("missing provider summary:\n%s", text)
	}
	if !strings.Contains(text, "Auth:       oauth token stored in auth store") {
		t.Fatalf("missing auth summary:\n%s", text)
	}
}
