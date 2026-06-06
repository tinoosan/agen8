package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	auth "github.com/tinoosan/agen8-mcp-server/internal/services/auth/domain"
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

// TestConcurrentSessionsResolveToOwnMember is the end-to-end proof of the
// disambiguation feature: two Claude Code conversations that share ONE link token
// must each resolve to their OWN member on a member-as-actor verb (here
// task.claim), not collide on a single shared identity.
//
// The realistic Claude scenario is modelled exactly: each conversation registers
// with only a session_id (its conversation id) - no thread_id, no transport
// Mcp-Session-Id header - because Claude Code's PreToolUse hook can only stamp
// session_id into the call's arguments. Resolution then runs through
// d.resolveMCPSession with an EMPTY http.Header, so the only identity signal is
// the in-band arguments.session_id. If the daemon ignored arguments on
// non-register verbs (the old behaviour) both calls would resolve member-less and
// this test would fail.
func TestConcurrentSessionsResolveToOwnMember(t *testing.T) {
	dataDir := t.TempDir()
	projectRoot := t.TempDir()
	rootA := t.TempDir()
	rootB := t.TempDir()
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

	// One initialize is enough for the stateless streamable handler to accept the
	// subsequent tool calls over this token.
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(linkToken), bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Two distinct conversations register under the SAME link token.
	memberA := registerSessionMemberForTest(t, handler, linkToken, rootA, "claude-sess-A", "Worker A")
	memberB := registerSessionMemberForTest(t, handler, linkToken, rootB, "claude-sess-B", "Worker B")
	if memberA == memberB {
		t.Fatalf("two sessions collided onto one member id %q", memberA)
	}

	ctx := context.Background()

	// A non-register verb (task.claim) carrying only arguments.session_id, with NO
	// transport header, must resolve to the matching member.
	gotA, err := d.resolveMCPSession(ctx, linkToken, http.Header{}, claimBodyForSession("claude-sess-A"))
	if err != nil {
		t.Fatalf("resolve session A: %v", err)
	}
	if gotA.MemberID != memberA {
		t.Fatalf("session A resolved to member %q want %q", gotA.MemberID, memberA)
	}
	gotB, err := d.resolveMCPSession(ctx, linkToken, http.Header{}, claimBodyForSession("claude-sess-B"))
	if err != nil {
		t.Fatalf("resolve session B: %v", err)
	}
	if gotB.MemberID != memberB {
		t.Fatalf("session B resolved to member %q want %q", gotB.MemberID, memberB)
	}

	// Body-over-header precedence (the resolveMCPSession flip): a stale transport
	// header naming the OTHER session must not override the fresh in-band id. With
	// concurrent conversations multiplexed over one connection, the header is an
	// unreliable per-call signal, so the body must win.
	staleHeader := http.Header{}
	staleHeader.Set("Agen8-Native-Session-Id", "claude-sess-B")
	gotMixed, err := d.resolveMCPSession(ctx, linkToken, staleHeader, claimBodyForSession("claude-sess-A"))
	if err != nil {
		t.Fatalf("resolve with mixed body/header: %v", err)
	}
	if gotMixed.MemberID != memberA {
		t.Fatalf("in-band session A lost to stale header: resolved %q want %q", gotMixed.MemberID, memberA)
	}
}

// registerSessionMemberForTest registers one MCP member under linkToken using only
// session_id (the Claude shape: a conversation id, no thread_id), and returns the
// created member id.
func registerSessionMemberForTest(t *testing.T, handler http.Handler, linkToken, callerRoot, sessionID, displayName string) string {
	t.Helper()
	registerBody := []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":2,
		"method":"tools/call",
		"params":{
			"name":"project",
			"arguments":{
				"action":"register",
				"project_root":%q,
				"display_name":%q,
				"harness_kind":"claude",
				"session_id":%q
			}
		}
	}`, callerRoot, displayName, sessionID))
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(linkToken), bytes.NewReader(registerBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("register error=%s", resp.Error.Message)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("register missing content: %s", rec.Body.String())
	}
	var registered struct {
		MemberID string `json:"memberId"`
	}
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &registered); err != nil {
		t.Fatalf("decode register content %q: %v", resp.Result.Content[0].Text, err)
	}
	if registered.MemberID == "" {
		t.Fatalf("register missing member id: %s", resp.Result.Content[0].Text)
	}
	return registered.MemberID
}

// claimBodyForSession builds a non-register tools/call (task.claim) whose only
// identity signal is arguments.session_id - the exact shape Claude Code's hook
// produces for a member-as-actor verb.
func claimBodyForSession(sessionID string) []byte {
	return []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":9,
		"method":"tools/call",
		"params":{
			"name":"task",
			"arguments":{"action":"claim","task_id":"task-1","session_id":%q}
		}
	}`, sessionID))
}

// TestAPIKeyResolvesConcurrentSessionMembers pins the shared-token concurrency
// case with the public setup token model: two Claude conversations sharing one
// user-scoped API key must each resolve to their own member on a non-register verb
// carrying only arguments.session_id. Resolution by native session ref must stay
// harness-agnostic.
func TestAPIKeyResolvesConcurrentSessionMembers(t *testing.T) {
	dataDir := t.TempDir()
	projectRoot := t.TempDir()
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

	apiKey := setupResult.APIKey.Secret
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(apiKey), bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Two Claude conversations register under the SAME user-scoped API key + SAME project.
	memberA := registerSessionMemberForTest(t, handler, apiKey, projectRoot, "claude-local-A", "Local Worker A")
	memberB := registerSessionMemberForTest(t, handler, apiKey, projectRoot, "claude-local-B", "Local Worker B")
	if memberA == memberB {
		t.Fatalf("two sessions collided onto one member id %q", memberA)
	}

	ctx := context.Background()
	gotA, err := d.resolveMCPSession(ctx, apiKey, http.Header{}, claimBodyForSession("claude-local-A"))
	if err != nil {
		t.Fatalf("resolve session A: %v", err)
	}
	if gotA.MemberID != memberA {
		t.Fatalf("session A resolved to member %q want %q", gotA.MemberID, memberA)
	}
	gotB, err := d.resolveMCPSession(ctx, apiKey, http.Header{}, claimBodyForSession("claude-local-B"))
	if err != nil {
		t.Fatalf("resolve session B: %v", err)
	}
	if gotB.MemberID != memberB {
		t.Fatalf("session B resolved to member %q want %q", gotB.MemberID, memberB)
	}
}

// TestResolveMCPSessionTokenClasses is the table-driven contract for how
// resolveMCPSession turns a raw bearer token into a session. It is the one place
// that lists every token class and its expected resolution outcome, so the failure
// paths and the success paths sit side by side.
//
// It pins three invariants on purpose:
//
//   - Unseeded token store, so resolution goes through the DB (dec-fb325eff). New()
//     builds its mcpTokens store with mcp.NewTokenStore() and never seeds it. On the
//     HTTP daemon path d.mcpTokens.Resolve therefore always misses, and every token
//     is resolved against the auth DB via HashToken -> GetByTokenHash. These cases
//     run through that real path. If a later change ever seeds mcpTokens, it would
//     short-circuit this and these assertions would stop exercising the DB.
//
//   - Member-less is success, not failure (dec-55648814). A valid token with no
//     in-band session ref, or with a ref that matches no member, resolves to a
//     member-LESS session with a nil error. That is the pre-registration affordance:
//     a brand-new conversation must reach project.register - the verb that creates
//     its member - before any member exists. Loud failure for member-less callers is
//     enforced downstream at each tool's actor gate (see the mission/graph/decision
//     handler member-less tests), not here.
//
//   - The two failure branches surface different errors, and that difference is the
//     proof of no fall-through. An invalid wlt_ is handled in the link-token branch
//     (daemon.go:271-275), which returns ValidateLinkToken's specific sentinel
//     (ErrTokenNotFound when unminted, ErrTokenExpired when revoked or expired). An
//     invalid ak_ is handled in the api-key branch (daemon.go:278-284), which returns
//     the original mcpTokens store-miss error and DISCARDS authErr (line 281) - so
//     every api-key failure collapses to the same generic error and the specific auth
//     sentinel never surfaces. Asserting the wlt_ sentinel therefore proves the token
//     was NOT silently retried down the api-key path: a fall-through would have yielded
//     the generic store-miss error instead. (The collapse on the api-key side is the
//     real behavior today; see dec-5c8aca7b for the asymmetry it leaves behind.)
//
// This subsumes the earlier single-purpose tests (invalid wlt_, invalid ak_, and
// unknown-session member-less) and adds the valid, revoked, expired, and empty
// classes.
func TestResolveMCPSessionTokenClasses(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectRoot := t.TempDir()
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

	// Setup yields both an admin session token (for ownership-gated RPCs) and a real
	// admin API key (an ak_ that resolves against the DB).
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
		APIKey struct {
			Secret string `json:"secret"`
		} `json:"apiKey"`
	}
	if err := json.Unmarshal(setupRec.Body.Bytes(), &setupResult); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	sessionToken := setupResult.Session.Token
	adminAPIKey := setupResult.APIKey.Secret
	if sessionToken == "" || adminAPIKey == "" {
		t.Fatalf("setup response missing tokens: session=%q apiKey=%q", sessionToken, adminAPIKey)
	}

	// The admin user id lets us mint extra keys directly through the auth service.
	adminUser, err := d.app.AuthSvc.ValidateAPIKey(ctx, adminAPIKey)
	if err != nil {
		t.Fatalf("resolve admin user: %v", err)
	}

	// A real project plus one registered member, so an unknown session ref resolves
	// to member.ErrNotFound (member-less) against a populated roster, not for some
	// unrelated reason.
	projectID := createProjectForLinkTokenTest(t, handler, sessionToken, projectRoot)

	// One initialize so the streamable MCP handler accepts the register tool call.
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(adminAPIKey), bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}
	if m := registerSessionMemberForTest(t, handler, adminAPIKey, projectRoot, "known-session", "Known Worker"); m == "" {
		t.Fatal("failed to register baseline member")
	}

	// A valid link token bound to the project (a real wlt_ that resolves via the DB).
	validLinkToken := createProjectLinkTokenForTest(t, handler, sessionToken, projectID)

	// Revoked and expired tokens are minted straight through the auth service so
	// resolveMCPSession hits the !IsActive branch (ErrTokenExpired) - a class the
	// not-found cases below never reach.
	past := time.Now().Add(-1 * time.Hour)
	revokedKey, err := d.app.AuthSvc.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{UserID: adminUser.ID, Name: "revoked-key"})
	if err != nil {
		t.Fatalf("create revoked api key: %v", err)
	}
	if err := d.app.AuthSvc.RevokeAPIKey(ctx, revokedKey.APIKey.ID); err != nil {
		t.Fatalf("revoke api key: %v", err)
	}
	expiredKey, err := d.app.AuthSvc.CreateAPIKey(ctx, authapp.CreateAPIKeyParams{UserID: adminUser.ID, Name: "expired-key", ExpiresAt: &past})
	if err != nil {
		t.Fatalf("create expired api key: %v", err)
	}
	revokedLink, err := d.app.AuthSvc.CreateLinkToken(ctx, authapp.CreateLinkTokenParams{UserID: adminUser.ID, ProjectID: projectID, Label: "revoked-link"})
	if err != nil {
		t.Fatalf("create revoked link token: %v", err)
	}
	if err := d.app.AuthSvc.RevokeLinkToken(ctx, revokedLink.LinkToken.ID); err != nil {
		t.Fatalf("revoke link token: %v", err)
	}
	expiredLink, err := d.app.AuthSvc.CreateLinkToken(ctx, authapp.CreateLinkTokenParams{UserID: adminUser.ID, ProjectID: projectID, Label: "expired-link", ExpiresAt: &past})
	if err != nil {
		t.Fatalf("create expired link token: %v", err)
	}

	// errClass names the error contract each token class must meet. It encodes WHERE the
	// failure is produced, which is how this table proves an invalid wlt_ is handled
	// inside the link-token branch and never falls through to the api-key path.
	type errClass int
	const (
		resolves             errClass = iota // success: nil error, usable member-less session
		authSentinelNotFound                 // wlt_ unminted: auth.ErrTokenNotFound surfaces from the link-token branch
		authSentinelExpired                  // wlt_ revoked or expired: auth.ErrTokenExpired surfaces from the link-token branch
		storeMissCollapsed                   // ak_ (any) or empty: collapses to the mcp store-miss error; authErr discarded at daemon.go:281
	)

	cases := []struct {
		name          string
		token         string
		body          []byte
		want          errClass
		wantProjectID string // asserted only on a success row when non-empty
	}{
		{
			name:  "valid api key, no session ref, resolves member-less",
			token: adminAPIKey,
			body:  nil,
			want:  resolves,
		},
		{
			name:          "valid link token, no session ref, resolves member-less bound to its project",
			token:         validLinkToken,
			body:          nil,
			want:          resolves,
			wantProjectID: projectID,
		},
		{
			name:  "valid api key, unknown session ref, stays member-less (dec-55648814)",
			token: adminAPIKey,
			body:  claimBodyForSession("session-that-was-never-registered"),
			want:  resolves,
		},
		// Every api-key failure collapses to the same mcp store-miss error: daemon.go:281
		// returns the original mcpTokens.Resolve error and discards authErr, so unminted,
		// revoked, and expired ak_ are indistinguishable to the caller.
		{name: "revoked api key collapses to store-miss error", token: revokedKey.Token, want: storeMissCollapsed},
		{name: "expired api key collapses to store-miss error", token: expiredKey.Token, want: storeMissCollapsed},
		{name: "unminted api key collapses to store-miss error", token: "ak_this-key-does-not-exist", want: storeMissCollapsed},
		{name: "empty token collapses to store-miss error", token: "", want: storeMissCollapsed},
		// Every link-token failure surfaces its specific auth sentinel from the wlt_ branch.
		// That specific sentinel is the proof of no fall-through: a fall-through to the
		// api-key path would instead yield the generic store-miss error asserted above.
		{name: "revoked link token surfaces ErrTokenExpired (no fall-through)", token: revokedLink.Token, want: authSentinelExpired},
		{name: "expired link token surfaces ErrTokenExpired (no fall-through)", token: expiredLink.Token, want: authSentinelExpired},
		{name: "unminted link token surfaces ErrTokenNotFound (no fall-through)", token: "wlt_this-token-was-never-minted", want: authSentinelNotFound},
	}

	// assertLoudFailure is the shared no-silent-fallback check: any failed resolve must
	// return a non-nil error AND a zero-value session, never a partial usable context.
	assertLoudFailure := func(t *testing.T, got mcp.Session, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("want a loud error, got nil (session=%+v)", got)
		}
		if got.Token != "" || got.UserID != "" || got.MemberID != "" || got.ProjectID != "" {
			t.Fatalf("failed resolve must yield a zero-value session, got %+v", got)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.resolveMCPSession(ctx, tc.token, http.Header{}, tc.body)
			switch tc.want {
			case authSentinelNotFound:
				assertLoudFailure(t, got, err)
				if !errors.Is(err, auth.ErrTokenNotFound) {
					t.Fatalf("invalid wlt_ must surface auth.ErrTokenNotFound from the link-token branch (proves no fall-through to the api-key path), got %v", err)
				}
				return
			case authSentinelExpired:
				assertLoudFailure(t, got, err)
				if !errors.Is(err, auth.ErrTokenExpired) {
					t.Fatalf("revoked/expired wlt_ must surface auth.ErrTokenExpired from the link-token branch (proves no fall-through), got %v", err)
				}
				return
			case storeMissCollapsed:
				assertLoudFailure(t, got, err)
				// authErr is discarded at daemon.go:281, so the specific auth sentinel must NOT
				// surface here - the failure collapses to the generic mcp store-miss error.
				if errors.Is(err, auth.ErrTokenNotFound) || errors.Is(err, auth.ErrTokenExpired) {
					t.Fatalf("api-key/empty failure should collapse to the mcp store-miss error (authErr discarded at daemon.go:281), but a domain sentinel surfaced: %v", err)
				}
				return
			}
			// tc.want == resolves: a valid token yields a usable session carrying the token...
			if err != nil {
				t.Fatalf("want success, got err: %v", err)
			}
			if got.Token == "" {
				t.Fatalf("resolved session missing token: %+v", got)
			}
			// ...but member-less: no member is bound without a matching registered session.
			if got.MemberID != "" {
				t.Fatalf("expected member-less resolution, got member %q", got.MemberID)
			}
			if tc.wantProjectID != "" && got.ProjectID != tc.wantProjectID {
				t.Fatalf("projectID=%q want %q", got.ProjectID, tc.wantProjectID)
			}
		})
	}
}

// decisionLogBodyForSession builds a non-register tools/call (decision.log) whose
// only identity signal is arguments.session_id - the exact shape Claude Code's hook
// produces for a member-as-actor verb. The wire layer strips session_id from the
// arguments (stripAmbientSessionRefs) before the decision tool's strict decoder runs,
// so this models a real Claude call rather than a hand-massaged one.
func decisionLogBodyForSession(sessionID, title, rationale string) []byte {
	return []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":10,
		"method":"tools/call",
		"params":{
			"name":"decision",
			"arguments":{"action":"log","title":%q,"rationale":%q,"session_id":%q}
		}
	}`, title, rationale, sessionID))
}

// TestMCPWireHappyPathForBoundMemberVerb is KR3's end-to-end happy path: one
// representative member-as-actor verb (decision.log) driven from a raw token all the
// way through the live HTTP /mcp pipeline - resolveMCPSession (token -> bound member)
// -> executeNativeMCPTool building the tool CallContext -> the decision tool's Handle
// -> the decision service -> a well-formed success response.
//
// It is the positive counterpart to the failure-path coverage, and it fills a real
// gap. The existing wire tests do not exercise this path: the register flow runs
// member-LESS by design (it is the verb that CREATES the member), and
// TestConcurrentSessionsResolveToOwnMember stops at d.resolveMCPSession for task.claim
// without ever driving the verb through a tool handler. Nothing else proves the
// pipeline composes for a NORMAL verb that REQUIRES a pre-bound member.
//
// decision.log is the right probe precisely because its handler hard-requires
// call.ActorMemberID (see internal/mcp/tools/decision/handler.go): it returns
// "member_id is required" when no actor is bound. So a 200 carrying a real decision id
// is only possible if the in-band arguments.session_id resolved to the registered
// member AND that member was threaded into the decision CallContext as the actor. A
// server-generated dec- id additionally proves the decision SERVICE actually ran, not
// a stub.
//
// Scope: happy path only. Loud failure on bad tokens is KR1
// (TestResolveMCPSessionTokenClasses); the binding gate's success-vs-contention split
// is KR2's task.claim work. There is deliberately no overlap here.
func TestMCPWireHappyPathForBoundMemberVerb(t *testing.T) {
	dataDir := t.TempDir()
	projectRoot := t.TempDir()
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

	// Setup yields a real admin API key (an ak_ that resolves against the DB). The
	// member registers under it, and the same key carries the later decision.log call.
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
	apiKey := setupResult.APIKey.Secret
	if apiKey == "" {
		t.Fatal("setup response missing api key secret")
	}

	// One initialize so the streamable MCP handler accepts the subsequent tool calls.
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(apiKey), bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Register the member whose session_id the verb will carry. Registration creates
	// the project from project_root, so the resolved session has a bound project too.
	const sessionID = "kr3-happy-session"
	memberID := registerSessionMemberForTest(t, handler, apiKey, projectRoot, sessionID, "KR3 Happy Worker")

	// Drive decision.log end to end over the wire, carrying only arguments.session_id.
	body := decisionLogBodyForSession(sessionID, "KR3 wire-path proof", "Driven end to end through /mcp to prove the pipeline composes for a bound-member verb.")
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(apiKey), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("decision.log status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode decision.log response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("decision.log rpc error=%s", resp.Error.Message)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("decision.log missing result content: %s", rec.Body.String())
	}

	// The content text is the decision tool's structured JSON. A server-generated dec-
	// id proves the decision service ran; memberId == the registered member proves the
	// resolved member was threaded through CallContext as the actor. decision.log
	// hard-requires an actor, so neither field could be satisfied by a member-less call.
	var logged struct {
		Decision struct {
			ID       string `json:"id"`
			MemberID string `json:"memberId"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &logged); err != nil {
		t.Fatalf("decode decision content %q: %v", resp.Result.Content[0].Text, err)
	}
	if !strings.HasPrefix(logged.Decision.ID, "dec-") {
		t.Fatalf("decision id=%q want dec- prefix (a server-generated id proves the service ran)", logged.Decision.ID)
	}
	if logged.Decision.MemberID != memberID {
		t.Fatalf("decision memberId=%q want %q (the resolved member must thread through CallContext as the actor)", logged.Decision.MemberID, memberID)
	}
}
