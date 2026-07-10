package daemon

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/rpc"
)

func TestSanitizeLoginResponseRemovesSessionToken(t *testing.T) {
	expiresAt := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	result, err := json.Marshal(map[string]any{
		"userId":    "user-a",
		"role":      "admin",
		"token":     "session-secret",
		"expiresAt": expiresAt,
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	raw, err := json.Marshal(rpc.Response{JSONRPC: "2.0", Result: result})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	sanitized, token, gotExpiry, ok := sanitizeLoginResponse(raw)
	if !ok || token != "session-secret" || !gotExpiry.Equal(expiresAt) {
		t.Fatalf("sanitize result ok=%v token=%q expiry=%s", ok, token, gotExpiry)
	}
	if strings.Contains(string(sanitized), "session-secret") || strings.Contains(string(sanitized), `"token"`) {
		t.Fatalf("sanitized response disclosed token: %s", sanitized)
	}
}

func TestSanitizeLoginResponseFailsClosedWithoutToken(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","result":{"userId":"user-a","expiresAt":"2026-07-11T12:00:00Z"}}`)
	if sanitized, _, _, ok := sanitizeLoginResponse(raw); ok || string(sanitized) != string(raw) {
		t.Fatalf("sanitize missing token ok=%v response=%s", ok, sanitized)
	}
}
