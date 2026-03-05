package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
)

type APIKeyProvider struct{}

func (p APIKeyProvider) Name() string { return ProviderAPIKey }

func (p APIKeyProvider) Login(ctx context.Context, interactive bool) error {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) != "" || strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return nil
	}
	return fmt.Errorf("API key login is not interactive here; set OPENROUTER_API_KEY or OPENAI_API_KEY")
}

func (p APIKeyProvider) Status(ctx context.Context) (Status, error) {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) != "" {
		return Status{Provider: ProviderAPIKey, LoggedIn: true, Source: "env:OPENROUTER_API_KEY"}, nil
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
		return Status{Provider: ProviderAPIKey, LoggedIn: true, Source: "env:OPENAI_API_KEY"}, nil
	}
	return Status{Provider: ProviderAPIKey, LoggedIn: false, Source: "env"}, nil
}

func (p APIKeyProvider) Logout(ctx context.Context) error {
	_ = os.Unsetenv("OPENROUTER_API_KEY")
	_ = os.Unsetenv("OPENAI_API_KEY")
	return nil
}

func (p APIKeyProvider) AccessToken(ctx context.Context) (Token, error) {
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if key == "" {
		return Token{}, fmt.Errorf("api key is missing")
	}
	return Token{AccessToken: key, ExpiresAt: time.Time{}}, nil
}

type Manager struct {
	provider Provider
}

func NewManager(dataDir, providerRaw string) (*Manager, error) {
	provider, err := ParseProvider(providerRaw)
	if err != nil {
		return nil, err
	}
	switch provider {
	case ProviderAPIKey:
		return &Manager{provider: APIKeyProvider{}}, nil
	case ProviderChatGPTAccount:
		cp, err := NewChatGPTAccountProvider(ChatGPTAccountProviderOptions{DataDir: dataDir})
		if err != nil {
			return nil, err
		}
		return &Manager{provider: cp}, nil
	default:
		return &Manager{provider: APIKeyProvider{}}, nil
	}
}

func NewManagerFromEnv(dataDir string) (*Manager, error) {
	if strings.TrimSpace(dataDir) == "" {
		resolved, err := config.ResolveDataDir("", false)
		if err != nil {
			return nil, err
		}
		dataDir = resolved
	}
	provider := NormalizeProvider(os.Getenv(EnvAuthProvider))
	return NewManager(dataDir, provider)
}

func (m *Manager) Provider() Provider {
	if m == nil {
		return nil
	}
	return m.provider
}
