package builtins

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestXOAuth2_Start_FormatsAuthString(t *testing.T) {
	auth := xoauth2Auth("user@gmail.com", "token123").(*xoauth2SMTPAuth)
	_, initial, err := auth.Start(nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// net/smtp base64-encodes the auth payload on the wire; Start returns the raw bytes.
	wire := base64.StdEncoding.EncodeToString(initial)
	b, _ := base64.StdEncoding.DecodeString(wire)
	got := string(b)
	want := "user=user@gmail.com\x01auth=Bearer token123\x01\x01"
	if got != want {
		t.Fatalf("auth payload mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestNewGmailOAuthClient_Defaults(t *testing.T) {
	c, err := NewGmailOAuthClient(GmailOAuthConfig{
		User:        "user@gmail.com",
		AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("NewGmailOAuthClient: %v", err)
	}
	if c.cfg.From != "user@gmail.com" {
		t.Fatalf("expected From to default to User, got %q", c.cfg.From)
	}
	if c.cfg.Host != "smtp.gmail.com" || c.cfg.Port != 587 {
		t.Fatalf("expected defaults smtp.gmail.com:587, got %s:%d", c.cfg.Host, c.cfg.Port)
	}
}

func TestSanitizeHeaderValue_StripsNewlines(t *testing.T) {
	got := sanitizeHeaderValue("a\r\nb\nc")
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("expected header to have no newlines, got %q", got)
	}
}
