package app

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if err := runInteractiveRuntimeOnboarding(dataDir, nil, nil); err != nil {
		t.Fatalf("runInteractiveRuntimeOnboarding: %v", err)
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
