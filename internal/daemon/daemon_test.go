package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
)

func TestSetupRejectsMismatchedPasswordConfirmation(t *testing.T) {
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: t.TempDir()},
		SetupToken: "test-setup-token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := d.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}

	form := "token=test-setup-token&email=admin%40example.com&name=Admin&password=password123&confirmPassword=password456"
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusBadRequest)
	}
	if body := rec.Body.String(); !strings.Contains(body, "password confirmation does not match") {
		t.Fatalf("body=%q want password confirmation mismatch error", body)
	}
	if open := d.setupAvailable(req.Context()); !open {
		t.Fatal("setup should remain open after rejected password confirmation")
	}
}
