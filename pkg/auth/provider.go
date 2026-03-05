package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	EnvAuthProvider = "AGEN8_AUTH_PROVIDER"

	ProviderAPIKey         = "api_key"
	ProviderChatGPTAccount = "chatgpt_account"
)

// Status describes provider login state without exposing secrets.
type Status struct {
	Provider  string
	LoggedIn  bool
	ExpiresAt time.Time
	AccountID string
	Source    string
}

// Token carries a usable access token for a provider request.
type Token struct {
	AccessToken string
	AccountID   string
	ExpiresAt   time.Time
}

// Provider defines the auth provider contract used by runtime + CLI.
type Provider interface {
	Name() string
	Login(ctx context.Context, interactive bool) error
	Status(ctx context.Context) (Status, error)
	Logout(ctx context.Context) error
	AccessToken(ctx context.Context) (Token, error)
}

func NormalizeProvider(raw string) string {
	p, err := ParseProvider(raw)
	if err != nil {
		return ProviderAPIKey
	}
	return p
}

func ParseProvider(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ProviderAPIKey:
		return ProviderAPIKey, nil
	case ProviderChatGPTAccount:
		return ProviderChatGPTAccount, nil
	default:
		return "", fmt.Errorf("unsupported auth provider %q (valid: %s, %s)", raw, ProviderAPIKey, ProviderChatGPTAccount)
	}
}
