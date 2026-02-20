package builtins

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// EmailSender is the minimal interface the host executor needs for sending emails.
type EmailSender interface {
	Send(to, subject, body string) error
}

type GmailOAuthConfig struct {
	// User is the Gmail address used for SMTP auth (e.g. "you@gmail.com").
	User string

	// From is the sender address used for SMTP envelope + header.
	// If empty, defaults to User.
	From string

	// OAuth2 client credentials (Google Cloud).
	ClientID     string
	ClientSecret string

	// RefreshToken is a long-lived token used to mint access tokens.
	RefreshToken string

	// Optional: if set, uses this access token directly (useful for debugging).
	// If empty, a token is minted using RefreshToken.
	AccessToken string

	// Optional: defaults to smtp.gmail.com:587 (STARTTLS).
	Host string
	Port int
}

type GmailOAuthClient struct {
	cfg         GmailOAuthConfig
	tokenSource oauth2.TokenSource
}

func NewGmailOAuthClient(cfg GmailOAuthConfig) (*GmailOAuthClient, error) {
	cfg.User = strings.TrimSpace(cfg.User)
	cfg.From = strings.TrimSpace(cfg.From)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.RefreshToken = strings.TrimSpace(cfg.RefreshToken)
	cfg.AccessToken = strings.TrimSpace(cfg.AccessToken)

	if cfg.User == "" {
		return nil, fmt.Errorf("gmail user is required")
	}
	if cfg.From == "" {
		cfg.From = cfg.User
	}

	if cfg.Host == "" {
		cfg.Host = "smtp.gmail.com"
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}

	// If the user provides an access token, allow using it directly for quick testing.
	if cfg.AccessToken != "" {
		return &GmailOAuthClient{cfg: cfg}, nil
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, fmt.Errorf("gmail oauth requires client id, client secret, and refresh token (or an access token)")
	}

	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{"https://mail.google.com/"},
	}
	ts := oauthCfg.TokenSource(context.Background(), &oauth2.Token{RefreshToken: cfg.RefreshToken})

	return &GmailOAuthClient{
		cfg:         cfg,
		tokenSource: oauth2.ReuseTokenSource(nil, ts),
	}, nil
}

func (c *GmailOAuthClient) Send(to, subject, body string) error {
	if c == nil {
		return fmt.Errorf("email client not configured")
	}

	from := sanitizeHeaderValue(strings.TrimSpace(c.cfg.From))
	to = sanitizeHeaderValue(strings.TrimSpace(to))
	subject = sanitizeHeaderValue(strings.TrimSpace(subject))
	if from == "" {
		return fmt.Errorf("from is required")
	}
	if to == "" {
		return fmt.Errorf("to is required")
	}
	if subject == "" {
		return fmt.Errorf("subject is required")
	}

	recipients := splitRecipients(to)
	if len(recipients) == 0 {
		return fmt.Errorf("to is required")
	}

	accessToken := strings.TrimSpace(c.cfg.AccessToken)
	if accessToken == "" {
		if c.tokenSource == nil {
			return fmt.Errorf("gmail oauth not configured (missing token source)")
		}
		tok, err := c.tokenSource.Token()
		if err != nil {
			return fmt.Errorf("gmail oauth token: %w", err)
		}
		accessToken = strings.TrimSpace(tok.AccessToken)
		if accessToken == "" {
			return fmt.Errorf("gmail oauth returned empty access token")
		}
		// Best-effort: avoid using a token that is about to expire.
		if !tok.Expiry.IsZero() && time.Until(tok.Expiry) < 10*time.Second {
			// Force refresh on next send.
			c.cfg.AccessToken = ""
		}
	}

	// Use CRLF line endings for SMTP.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")

	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/plain; charset=\"utf-8\"\r\n"+
			"\r\n"+
			"%s\r\n",
		from, to, subject, body,
	))

	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)

	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: c.cfg.Host}); err != nil {
		return err
	}

	auth := xoauth2Auth(c.cfg.User, accessToken)
	if err := client.Auth(auth); err != nil {
		return err
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func splitRecipients(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sanitizeHeaderValue(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}

type xoauth2SMTPAuth struct {
	username string
	token    string
}

func xoauth2Auth(username, accessToken string) smtp.Auth {
	return &xoauth2SMTPAuth{
		username: strings.TrimSpace(username),
		token:    strings.TrimSpace(accessToken),
	}
}

func (a *xoauth2SMTPAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	if a == nil || a.username == "" || a.token == "" {
		return "", nil, fmt.Errorf("xoauth2 auth requires username and token")
	}
	// NOTE: net/smtp handles the base64 encoding of the initial response.
	// XOAUTH2 requires the encoded payload to be:
	// base64("user=<email>\x01auth=Bearer <token>\x01\x01")
	raw := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.token)
	return "XOAUTH2", []byte(raw), nil
}

func (a *xoauth2SMTPAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	// Gmail typically returns a base64-encoded JSON error payload here.
	// Return an error including the raw payload to aid debugging.
	payload := strings.TrimSpace(string(fromServer))
	if payload == "" {
		return nil, errors.New("gmail xoauth2: authentication rejected")
	}
	return nil, fmt.Errorf("gmail xoauth2: authentication rejected: %s", payload)
}
