package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestProjectLinkTokenCanRegisterBoundMCPContext(t *testing.T) {
	dataDir := t.TempDir()
	projectRoot := t.TempDir()
	callerRoot := t.TempDir()
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: dataDir},
		SetupToken: "test-setup-token",
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
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
	}
	if err := json.Unmarshal(setupRec.Body.Bytes(), &setupResult); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	if setupResult.Session.Token == "" {
		t.Fatal("setup response missing session token")
	}

	projectID := createProjectForLinkTokenTest(t, handler, setupResult.Session.Token, projectRoot)
	linkToken := createProjectLinkTokenForTest(t, handler, setupResult.Session.Token, projectID)

	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(linkToken), bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	registerBody := []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":2,
		"method":"tools/call",
		"params":{
			"name":"project",
			"arguments":{
				"action":"register",
				"project_root":%q,
				"project_id":"spoofed-project",
				"display_name":"Link token worker",
				"harness_kind":"codex",
				"session_id":"test-session",
				"thread_id":"test-thread"
			}
		}
	}`, callerRoot))
	registerReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(linkToken), bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerReq.Header.Set("Accept", "application/json, text/event-stream")
	registerRec := httptest.NewRecorder()
	handler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusOK {
		t.Fatalf("mcp register status=%d body=%s", registerRec.Code, registerRec.Body.String())
	}
	var registerResp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(registerRec.Body.Bytes(), &registerResp); err != nil {
		t.Fatalf("decode mcp register response: %v", err)
	}
	if registerResp.Error != nil {
		t.Fatalf("mcp register error=%s", registerResp.Error.Message)
	}
	if len(registerResp.Result.Content) == 0 {
		t.Fatalf("mcp register missing content: %s", registerRec.Body.String())
	}
	var registered struct {
		ProjectID string `json:"projectId"`
		MemberID  string `json:"memberId"`
		Token     string `json:"token"`
	}
	if err := json.Unmarshal([]byte(registerResp.Result.Content[0].Text), &registered); err != nil {
		t.Fatalf("decode register content %q: %v", registerResp.Result.Content[0].Text, err)
	}
	if registered.ProjectID != projectID {
		t.Fatalf("registered project=%q want bound project %q", registered.ProjectID, projectID)
	}
	if registered.MemberID == "" {
		t.Fatalf("registered response missing member id: %+v", registered)
	}
	if registered.Token != linkToken {
		t.Fatalf("registered token changed unexpectedly")
	}
}

func createProjectForLinkTokenTest(t *testing.T, handler http.Handler, sessionToken string, root string) string {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"project.create","params":{"root":%q,"title":"Link Token Project"}}`, root))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project.create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Project struct {
				ID string `json:"id"`
			} `json:"project"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode project.create response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("project.create error=%s", resp.Error.Message)
	}
	if resp.Result.Project.ID == "" {
		t.Fatalf("project.create missing project id: %s", rec.Body.String())
	}
	return resp.Result.Project.ID
}

func createProjectLinkTokenForTest(t *testing.T, handler http.Handler, sessionToken string, projectID string) string {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"project.linkToken.create","params":{"projectId":%q,"label":"uat"}}`, projectID))
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project.linkToken.create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode project.linkToken.create response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("project.linkToken.create error=%s", resp.Error.Message)
	}
	if !strings.HasPrefix(resp.Result.Token, "wlt_") {
		t.Fatalf("link token=%q want wlt_ prefix", resp.Result.Token)
	}
	return resp.Result.Token
}
