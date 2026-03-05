package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
)

const (
	DefaultOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	AuthorizeURL         = "https://auth.openai.com/oauth/authorize"
	TokenURL             = "https://auth.openai.com/oauth/token"
	DefaultRedirectURL   = "http://localhost:1455/auth/callback"
	DefaultScope         = "openid profile email offline_access"
)

var ErrAuthRequired = errors.New("chatgpt account authentication is required")

type ChatGPTAccountProviderOptions struct {
	DataDir     string
	ClientID    string
	RedirectURL string
	Scope       string
	HTTPClient  *http.Client
	In          io.Reader
	Out         io.Writer
	OpenBrowser func(string) error
	Now         func() time.Time
}

type ChatGPTAccountProvider struct {
	store       *FileTokenStore
	clientID    string
	redirectURL string
	scope       string
	httpClient  *http.Client
	in          io.Reader
	out         io.Writer
	openBrowser func(string) error
	now         func() time.Time
}

func NewChatGPTAccountProvider(opts ChatGPTAccountProviderOptions) (*ChatGPTAccountProvider, error) {
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		resolved, err := config.ResolveDataDir("", false)
		if err != nil {
			return nil, err
		}
		dataDir = resolved
	}
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data dir is required")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		clientID = DefaultOAuthClientID
	}
	redirectURL := strings.TrimSpace(opts.RedirectURL)
	if redirectURL == "" {
		redirectURL = DefaultRedirectURL
	}
	scope := strings.TrimSpace(opts.Scope)
	if scope == "" {
		scope = DefaultScope
	}
	openFn := opts.OpenBrowser
	if openFn == nil {
		openFn = OpenBrowser
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &ChatGPTAccountProvider{
		store:       NewFileTokenStore(dataDir),
		clientID:    clientID,
		redirectURL: redirectURL,
		scope:       scope,
		httpClient:  client,
		in:          opts.In,
		out:         opts.Out,
		openBrowser: openFn,
		now:         nowFn,
	}, nil
}

func (p *ChatGPTAccountProvider) Name() string {
	return ProviderChatGPTAccount
}

func (p *ChatGPTAccountProvider) Status(ctx context.Context) (Status, error) {
	rec, err := p.store.Load()
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return Status{Provider: p.Name(), LoggedIn: false, Source: filepath.Clean(p.store.Path())}, nil
		}
		return Status{}, err
	}
	return Status{
		Provider:  p.Name(),
		LoggedIn:  strings.TrimSpace(rec.AccessToken) != "" && strings.TrimSpace(rec.RefreshToken) != "",
		ExpiresAt: rec.ExpiresAt(),
		AccountID: strings.TrimSpace(rec.AccountID),
		Source:    filepath.Clean(p.store.Path()),
	}, nil
}

func (p *ChatGPTAccountProvider) Logout(ctx context.Context) error {
	return p.store.Delete()
}

func (p *ChatGPTAccountProvider) AccessToken(ctx context.Context) (Token, error) {
	rec, err := p.store.Load()
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return Token{}, ErrAuthRequired
		}
		return Token{}, err
	}
	now := p.now()
	expiresAt := rec.ExpiresAt()
	if strings.TrimSpace(rec.AccessToken) == "" || strings.TrimSpace(rec.RefreshToken) == "" || expiresAt.IsZero() {
		return Token{}, ErrAuthRequired
	}
	if !expiresAt.After(now.Add(60 * time.Second)) {
		refreshed, err := p.refreshToken(ctx, rec.RefreshToken)
		if err != nil {
			return Token{}, fmt.Errorf("%w: %v", ErrAuthRequired, err)
		}
		accountID := strings.TrimSpace(rec.AccountID)
		if aid, err := extractAccountID(refreshed.AccessToken); err == nil && strings.TrimSpace(aid) != "" {
			accountID = aid
		}
		rec = OAuthTokenRecord{
			Provider:      ProviderChatGPTAccount,
			AccessToken:   refreshed.AccessToken,
			RefreshToken:  refreshed.RefreshToken,
			ExpiresAtUnix: refreshed.ExpiresAt.UnixMilli(),
			AccountID:     accountID,
			TokenType:     "Bearer",
		}
		if err := p.store.Save(rec); err != nil {
			return Token{}, err
		}
	}
	return Token{AccessToken: rec.AccessToken, AccountID: rec.AccountID, ExpiresAt: rec.ExpiresAt()}, nil
}

func (p *ChatGPTAccountProvider) Login(ctx context.Context, interactive bool) error {
	pkceVerifier, pkceChallenge, err := generatePKCE()
	if err != nil {
		return err
	}
	state, err := createState()
	if err != nil {
		return err
	}
	authURL, err := p.buildAuthorizeURL(state, pkceChallenge)
	if err != nil {
		return err
	}
	server, err := StartOAuthCallbackServer(state)
	if err != nil {
		return err
	}
	defer server.Close()

	if interactive && p.out != nil {
		_, _ = fmt.Fprintf(p.out, "Open this URL to authenticate:\n%s\n\n", authURL)
	}
	if interactive && p.openBrowser != nil {
		_ = p.openBrowser(authURL)
	}

	var code string
	if server.Ready() {
		waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if got, werr := server.WaitForCode(waitCtx); werr == nil && strings.TrimSpace(got) != "" {
			code = strings.TrimSpace(got)
		}
	}
	if code == "" {
		if !interactive {
			return ErrAuthRequired
		}
		manual, merr := p.readManualCode(authURL, state)
		if merr != nil {
			return merr
		}
		code = manual
	}
	resp, err := p.exchangeCode(ctx, code, pkceVerifier)
	if err != nil {
		return err
	}
	accountID, err := extractAccountID(resp.AccessToken)
	if err != nil {
		return err
	}
	rec := OAuthTokenRecord{
		Provider:      ProviderChatGPTAccount,
		AccessToken:   resp.AccessToken,
		RefreshToken:  resp.RefreshToken,
		ExpiresAtUnix: resp.ExpiresAt.UnixMilli(),
		AccountID:     accountID,
		TokenType:     "Bearer",
	}
	if err := p.store.Save(rec); err != nil {
		return err
	}
	return nil
}

func (p *ChatGPTAccountProvider) buildAuthorizeURL(state, challenge string) (string, error) {
	u, err := url.Parse(AuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", p.redirectURL)
	q.Set("scope", p.scope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "codex_cli_rs")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (p *ChatGPTAccountProvider) readManualCode(authURL string, expectedState string) (string, error) {
	if p.out != nil {
		_, _ = fmt.Fprintf(p.out, "Automatic callback failed. After login, paste redirect URL or code:\n")
		_, _ = fmt.Fprintf(p.out, "%s\n", authURL)
	}
	reader := bufio.NewReader(os.Stdin)
	if p.in != nil {
		reader = bufio.NewReader(p.in)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("authorization code is required")
	}
	code, state := parseAuthorizationInput(line)
	expectedState = strings.TrimSpace(expectedState)
	state = strings.TrimSpace(state)
	if expectedState != "" && state != "" && state != expectedState {
		return "", fmt.Errorf("oauth state mismatch")
	}
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("authorization code is required")
	}
	return strings.TrimSpace(code), nil
}

func parseAuthorizationInput(input string) (code, state string) {
	v := strings.TrimSpace(input)
	if v == "" {
		return "", ""
	}
	if u, err := url.Parse(v); err == nil && strings.TrimSpace(u.Scheme) != "" {
		q := u.Query()
		return strings.TrimSpace(q.Get("code")), strings.TrimSpace(q.Get("state"))
	}
	if strings.Contains(v, "code=") {
		q, _ := url.ParseQuery(v)
		return strings.TrimSpace(q.Get("code")), strings.TrimSpace(q.Get("state"))
	}
	if strings.Contains(v, "#") {
		parts := strings.SplitN(v, "#", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return v, ""
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type oauthTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func (p *ChatGPTAccountProvider) exchangeCode(ctx context.Context, code, verifier string) (oauthTokenResult, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", p.clientID)
	values.Set("code", strings.TrimSpace(code))
	values.Set("code_verifier", strings.TrimSpace(verifier))
	values.Set("redirect_uri", p.redirectURL)
	return p.postToken(ctx, values)
}

func (p *ChatGPTAccountProvider) refreshToken(ctx context.Context, refreshToken string) (oauthTokenResult, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("client_id", p.clientID)
	values.Set("refresh_token", strings.TrimSpace(refreshToken))
	return p.postToken(ctx, values)
}

func (p *ChatGPTAccountProvider) postToken(ctx context.Context, form url.Values) (oauthTokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return oauthTokenResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return oauthTokenResult{}, fmt.Errorf(
			"oauth token request failed: status=%d body=%s",
			resp.StatusCode,
			sanitizeTokenErrorBody(strings.TrimSpace(string(b))),
		)
	}
	var body oauthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return oauthTokenResult{}, err
	}
	if strings.TrimSpace(body.AccessToken) == "" || strings.TrimSpace(body.RefreshToken) == "" || body.ExpiresIn <= 0 {
		return oauthTokenResult{}, fmt.Errorf("oauth token response missing fields")
	}
	return oauthTokenResult{
		AccessToken:  strings.TrimSpace(body.AccessToken),
		RefreshToken: strings.TrimSpace(body.RefreshToken),
		ExpiresAt:    p.now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}, nil
}

func createState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generatePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

func extractAccountID(accessToken string) (string, error) {
	parts := strings.Split(strings.TrimSpace(accessToken), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid access token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return "", err
	}
	if id, _ := raw["https://api.openai.com/auth.chatgpt_account_id"].(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	claim, _ := raw["https://api.openai.com/auth"].(map[string]any)
	id, _ := claim["chatgpt_account_id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("account id claim missing in access token")
	}
	return id, nil
}

func sanitizeTokenErrorBody(body string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	redacted := body
	for _, field := range []string{"access_token", "refresh_token"} {
		for {
			keyPos := strings.Index(redacted, `"`+field+`"`)
			if keyPos < 0 {
				break
			}
			colonPos := strings.Index(redacted[keyPos:], ":")
			if colonPos < 0 {
				break
			}
			valueStart := keyPos + colonPos + 1
			firstQuote := strings.Index(redacted[valueStart:], `"`)
			if firstQuote < 0 {
				break
			}
			firstQuote += valueStart
			secondQuote := strings.Index(redacted[firstQuote+1:], `"`)
			if secondQuote < 0 {
				break
			}
			secondQuote += firstQuote + 1
			redacted = redacted[:firstQuote+1] + "<redacted>" + redacted[secondQuote:]
		}
	}
	return redacted
}
