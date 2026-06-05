package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/pkg/buildinfo"
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

func TestHealthzIncludesBuildInfo(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildDate = oldBuildDate
	})
	buildinfo.Version = "v0.1.0"
	buildinfo.Commit = "abc1234"
	buildinfo.BuildDate = "2026-06-05T19:30:00Z"

	d, err := New(Config{
		AppConfig: config.Config{DataDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()
	d.handleHealthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		OK        bool   `json:"ok"`
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildDate string `json:"buildDate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if !body.OK || body.Version != "v0.1.0" || body.Commit != "abc1234" || body.BuildDate != "2026-06-05T19:30:00Z" {
		t.Fatalf("healthz body=%+v", body)
	}
}

func TestSetupAPIKeyWorksAsMCPTokenAfterRestart(t *testing.T) {
	dataDir := t.TempDir()
	setupToken := "test-setup-token"
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: dataDir},
		SetupToken: setupToken,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := d.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}

	setupBody := `{"token":"test-setup-token","email":"admin@example.com","name":"Admin","password":"password123","confirmPassword":"password123"}`
	setupReq := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(setupBody))
	setupReq.Header.Set("Content-Type", "application/json")
	setupReq.Header.Set("Accept", "application/json")
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status=%d body=%s", setupRec.Code, setupRec.Body.String())
	}
	var setupResult struct {
		APIKey struct {
			Secret string `json:"secret"`
		} `json:"apiKey"`
	}
	if err := json.Unmarshal(setupRec.Body.Bytes(), &setupResult); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	if setupResult.APIKey.Secret == "" {
		t.Fatal("setup response missing api key secret")
	}

	restarted, err := New(Config{
		AppConfig:  config.Config{DataDir: dataDir},
		SetupToken: "new-setup-token",
	})
	if err != nil {
		t.Fatalf("restart New: %v", err)
	}
	restartedHandler, err := restarted.httpHandler()
	if err != nil {
		t.Fatalf("restart httpHandler: %v", err)
	}

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+setupResult.APIKey.Secret, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	restartedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("mcp status=%d body=%s", rec.Code, rec.Body.String())
	}
	responseBody := rec.Body.String()
	if strings.Contains(responseBody, "not registered") || strings.Contains(responseBody, "token is required") {
		t.Fatalf("mcp rejected setup api key after restart: %s", responseBody)
	}
	if !strings.Contains(responseBody, "agen8-mcp") {
		t.Fatalf("mcp initialize body=%s want agen8-mcp server info", responseBody)
	}
}
