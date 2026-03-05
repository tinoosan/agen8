package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

var ErrChatGPTAccountLoginRequired = errors.New("chatgpt_account login is required")

var chatGPTCodexReadinessURL = "https://chatgpt.com/backend-api/codex/responses"
var chatGPTCodexReadinessHTTPClient = &http.Client{Timeout: 8 * time.Second}

// EnsureRuntimeAuthReady verifies authentication prerequisites for the selected
// runtime auth provider. For chatgpt_account, this ensures a usable access token
// is available before session/coordinator entrypoints proceed.
func EnsureRuntimeAuthReady(ctx context.Context, dataDir, providerRaw string) error {
	if strings.TrimSpace(providerRaw) == "" {
		providerRaw = strings.TrimSpace(os.Getenv(authpkg.EnvAuthProvider))
	}
	provider, err := authpkg.ParseProvider(providerRaw)
	if err != nil {
		return err
	}
	if provider != authpkg.ProviderChatGPTAccount {
		return nil
	}
	mgr, err := authpkg.NewManager(dataDir, provider)
	if err != nil {
		return err
	}
	p := mgr.Provider()
	if p == nil {
		return fmt.Errorf("%w: run `agen8 auth login --provider chatgpt_account`", ErrChatGPTAccountLoginRequired)
	}
	tok, err := p.AccessToken(ctx)
	if err != nil {
		if errors.Is(err, authpkg.ErrAuthRequired) {
			return fmt.Errorf("%w: run `agen8 auth login --provider chatgpt_account`", ErrChatGPTAccountLoginRequired)
		}
		return fmt.Errorf("%w: run `agen8 auth login --provider chatgpt_account` (%v)", ErrChatGPTAccountLoginRequired, err)
	}
	if strings.TrimSpace(tok.AccessToken) == "" || strings.TrimSpace(tok.AccountID) == "" {
		return fmt.Errorf("%w: run `agen8 auth login --provider chatgpt_account`", ErrChatGPTAccountLoginRequired)
	}
	if err := probeChatGPTCodexReadiness(ctx, tok); err != nil {
		return fmt.Errorf("%w: run `agen8 auth login --provider chatgpt_account` (%v)", ErrChatGPTAccountLoginRequired, err)
	}
	return nil
}

func probeChatGPTCodexReadiness(ctx context.Context, tok authpkg.Token) error {
	url := strings.TrimSpace(chatGPTCodexReadinessURL)
	if url == "" {
		return nil
	}
	client := chatGPTCodexReadinessHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build readiness probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tok.AccessToken))
	req.Header.Set("chatgpt-account-id", strings.TrimSpace(tok.AccountID))
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("chatgpt backend readiness probe failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(body))

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		if detail != "" {
			return fmt.Errorf("chatgpt auth rejected readiness probe (status=%d detail=%s)", resp.StatusCode, detail)
		}
		return fmt.Errorf("chatgpt auth rejected readiness probe (status=%d)", resp.StatusCode)
	case http.StatusNotFound:
		if detail != "" {
			return fmt.Errorf("chatgpt codex route unavailable (status=%d detail=%s)", resp.StatusCode, detail)
		}
		return fmt.Errorf("chatgpt codex route unavailable (status=%d)", resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		if detail != "" {
			return fmt.Errorf("chatgpt backend unavailable (status=%d detail=%s)", resp.StatusCode, detail)
		}
		return fmt.Errorf("chatgpt backend unavailable (status=%d)", resp.StatusCode)
	}
	// Any non-auth 2xx/4xx response indicates route reachability + accepted credentials.
	return nil
}
