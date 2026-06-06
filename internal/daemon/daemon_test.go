package daemon

import (
	"bytes"
	"context"
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

// TestBootstrapTokenResolvesConcurrentSessionMembers is the agen8-local mirror of
// TestConcurrentSessionsResolveToOwnMember, and it pins the bug the live dogfood
// exposed. The agen8-local bootstrap token is shared by EVERY harness, yet its
// in-memory session is hardcoded HarnessKind="codex" (registerBootstrapMCPToken).
// Two Claude conversations sharing agen8-local must still each resolve to their OWN
// member on a non-register verb carrying only arguments.session_id. If the token's
// "codex" harness were used to filter the lookup, a member registered as "claude"
// would be invisible and the caller would resolve member-less - which is exactly
// what happened against the live daemon: registration disambiguated the two members
// but task.list returned "registered member_id is required". Resolution by native
// session ref must be harness-agnostic.
func TestBootstrapTokenResolvesConcurrentSessionMembers(t *testing.T) {
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

	// agen8-local is registered by New (registerBootstrapMCPToken); no link token is
	// minted. This is the real Claude-on-bootstrap shape.
	const bootstrapToken = "agen8-local"
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+bootstrapToken, bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Two Claude conversations register under the SAME bootstrap token + SAME project.
	memberA := registerSessionMemberForTest(t, handler, bootstrapToken, projectRoot, "claude-local-A", "Local Worker A")
	memberB := registerSessionMemberForTest(t, handler, bootstrapToken, projectRoot, "claude-local-B", "Local Worker B")
	if memberA == memberB {
		t.Fatalf("two sessions collided onto one member id %q", memberA)
	}

	ctx := context.Background()
	gotA, err := d.resolveMCPSession(ctx, bootstrapToken, http.Header{}, claimBodyForSession("claude-local-A"))
	if err != nil {
		t.Fatalf("resolve session A: %v", err)
	}
	if gotA.MemberID != memberA {
		t.Fatalf("session A resolved to member %q want %q (bootstrap harness must not shadow claude member)", gotA.MemberID, memberA)
	}
	gotB, err := d.resolveMCPSession(ctx, bootstrapToken, http.Header{}, claimBodyForSession("claude-local-B"))
	if err != nil {
		t.Fatalf("resolve session B: %v", err)
	}
	if gotB.MemberID != memberB {
		t.Fatalf("session B resolved to member %q want %q", gotB.MemberID, memberB)
	}
}

// TestUnknownSessionResolvesMemberlessWithoutError locks the pre-registration
// affordance that the reliability inventory's finding #1 mistook for a silent
// fallback to convert into a loud error. resolveMCPSession deliberately returns a
// member-LESS session with NO error when an in-band session ref matches no member:
// a brand-new conversation must be able to reach project.register - the verb that
// CREATES its member - before any member exists. Turning this into a loud error
// would make the very first register call impossible (chicken-and-egg). Loud
// failure for member-less callers is enforced DOWNSTREAM at each tool's actor gate
// (see the mission/graph/decision handler member-less tests), not here.
//
// The in-band ref must be present (otherwise the no-refs early return fires for an
// unrelated reason); ResolveMCPContext returns member.ErrNotFound for the unknown
// session, and resolveMCPSession must translate that into a member-less session.
func TestUnknownSessionResolvesMemberlessWithoutError(t *testing.T) {
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

	const bootstrapToken = "agen8-local"
	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?token="+bootstrapToken, bytes.NewReader(initializeBody))
	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("Accept", "application/json, text/event-stream")
	initRec := httptest.NewRecorder()
	handler.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Establish a real project + roster under the bootstrap token so the only thing
	// missing during resolution is the specific session's member - proving an unknown
	// session is member-less even against a populated roster.
	known := registerSessionMemberForTest(t, handler, bootstrapToken, projectRoot, "claude-local-known", "Known Worker")
	if known == "" {
		t.Fatal("failed to register baseline member")
	}

	session, err := d.resolveMCPSession(context.Background(), bootstrapToken, http.Header{}, claimBodyForSession("claude-local-UNREGISTERED"))
	if err != nil {
		t.Fatalf("unknown session must resolve member-less without error, got err=%v", err)
	}
	if session.MemberID != "" {
		t.Fatalf("unknown session resolved to member %q, want member-less", session.MemberID)
	}
}
