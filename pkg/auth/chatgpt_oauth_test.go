package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func makeJWT(accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
		},
	}
	payloadRaw, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadRaw)
	return header + "." + payload + ".sig"
}

func TestBuildAuthorizeURLIncludesExpectedParams(t *testing.T) {
	p, err := NewChatGPTAccountProvider(ChatGPTAccountProviderOptions{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	u, err := p.buildAuthorizeURL("state123", "challenge123")
	if err != nil {
		t.Fatalf("build url: %v", err)
	}
	for _, want := range []string{
		"response_type=code",
		"client_id=" + DefaultOAuthClientID,
		"state=state123",
		"code_challenge=challenge123",
		"code_challenge_method=S256",
		"codex_cli_simplified_flow=true",
		"originator=codex_cli_rs",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("url missing %q: %s", want, u)
		}
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	code, state := parseAuthorizationInput("https://example/callback?code=abc&state=xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("unexpected parse: %q %q", code, state)
	}
	code, state = parseAuthorizationInput("abc#xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("unexpected parse hash: %q %q", code, state)
	}
}

func TestAccessToken_RefreshesWhenExpired(t *testing.T) {
	now := time.Now().UTC()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != TokenURL {
			t.Fatalf("unexpected URL: %s", r.URL.String())
		}
		if got := r.Method; got != http.MethodPost {
			t.Fatalf("method=%s", got)
		}
		respBody := `{"access_token":"` + makeJWT("acct_new") + `","refresh_token":"new_refresh","expires_in":3600}`
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(respBody)),
		}, nil
	})
	p, err := NewChatGPTAccountProvider(ChatGPTAccountProviderOptions{
		DataDir:    t.TempDir(),
		Now:        func() time.Time { return now },
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if err := p.store.Save(OAuthTokenRecord{
		Provider:      ProviderChatGPTAccount,
		AccessToken:   makeJWT("acct_old"),
		RefreshToken:  "old_refresh",
		ExpiresAtUnix: now.Add(-time.Minute).UnixMilli(),
		AccountID:     "acct_old",
	}); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	tok, err := p.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("access token: %v", err)
	}
	if tok.AccountID != "acct_new" {
		t.Fatalf("account id=%q", tok.AccountID)
	}
	loaded, err := p.store.Load()
	if err != nil {
		t.Fatalf("load after refresh: %v", err)
	}
	if loaded.RefreshToken != "new_refresh" {
		t.Fatalf("refresh token not updated: %+v", loaded)
	}
}

func TestLogin_ManualCodeFallback(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	ln, err := net.Listen("tcp", OAuthCallbackAddr)
	if err != nil {
		t.Fatalf("listen callback addr: %v", err)
	}
	defer ln.Close()
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		respBody := `{"access_token":"` + makeJWT("acct_manual") + `","refresh_token":"r2","expires_in":1200}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(respBody))}, nil
	})
	p, err := NewChatGPTAccountProvider(ChatGPTAccountProviderOptions{
		DataDir:     t.TempDir(),
		Now:         func() time.Time { return now },
		HTTPClient:  &http.Client{Transport: transport},
		OpenBrowser: func(string) error { return nil },
		In:          bytes.NewBufferString("https://localhost/callback?code=my-code\n"),
		Out:         io.Discard,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if err := p.Login(ctx, true); err != nil {
		t.Fatalf("login: %v", err)
	}
	status, err := p.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.LoggedIn || status.AccountID != "acct_manual" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestLogin_ManualCodeFallback_RejectsStateMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	ln, err := net.Listen("tcp", OAuthCallbackAddr)
	if err != nil {
		t.Fatalf("listen callback addr: %v", err)
	}
	defer ln.Close()
	p, err := NewChatGPTAccountProvider(ChatGPTAccountProviderOptions{
		DataDir:     t.TempDir(),
		Now:         func() time.Time { return now },
		HTTPClient:  &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, nil })},
		OpenBrowser: func(string) error { return nil },
		In:          bytes.NewBufferString("https://localhost/callback?code=my-code&state=bad-state\n"),
		Out:         io.Discard,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	err = p.Login(ctx, true)
	if err == nil {
		t.Fatalf("expected state mismatch error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "state mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractAccountID(t *testing.T) {
	id, err := extractAccountID(makeJWT("acct_77"))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if id != "acct_77" {
		t.Fatalf("id=%q", id)
	}
}

func TestExtractAccountID_FlatClaimFallback(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadRaw := []byte(`{"https://api.openai.com/auth.chatgpt_account_id":"acct_flat"}`)
	token := header + "." + base64.RawURLEncoding.EncodeToString(payloadRaw) + ".sig"
	id, err := extractAccountID(token)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if id != "acct_flat" {
		t.Fatalf("id=%q", id)
	}
}
