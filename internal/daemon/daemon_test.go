package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/mcp"
	"github.com/tinoosan/agen8/internal/rpc"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	auth "github.com/tinoosan/agen8/internal/services/auth/domain"
	"github.com/tinoosan/agen8/pkg/buildinfo"
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

func TestHandleRPCRejectsOverlyLargePayload(t *testing.T) {
	d, err := New(Config{AppConfig: config.Config{DataDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := d.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}
	payload := `{"jsonrpc":"2.0","id":1,"method":"auth.login","params":{"username":"` + strings.Repeat("a", maxRPCRequestBodyBytes) + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(payload))
	req.ContentLength = int64(maxRPCRequestBodyBytes + 1)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "request body is too large") {
		t.Fatalf("body=%q want 'request body is too large'", rec.Body.String())
	}
}

func TestReadRequestBodyRejectsChunkedOverLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", maxRPCRequestBodyBytes+1)))
	req.ContentLength = -1

	body, err := readRequestBody(req, maxRPCRequestBodyBytes)
	if err == nil {
		t.Fatalf("want error for oversized chunked payload, got body=%q", body)
	}
	if !strings.Contains(err.Error(), "rpc request body is too large") {
		t.Fatalf("got error=%v, want too-large error", err)
	}
}

func TestReadRequestBodyRestoresBodyForReuse(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"auth.login"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))

	body, err := readRequestBody(req, maxRPCRequestBodyBytes)
	if err != nil {
		t.Fatalf("readRequestBody: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("body=%q want %q", body, payload)
	}
	reread, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("re-read restored body: %v", err)
	}
	if !bytes.Equal(reread, payload) {
		t.Fatalf("restored body=%q want %q", reread, payload)
	}
}

func TestHandleSetupRejectsOversizedJSONPayload(t *testing.T) {
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
	oversizedName := strings.Repeat("a", maxSetupRequestBodyBytes)
	setupBody := `{"token":"test-setup-token","email":"admin@example.com","name":"` + oversizedName + `","password":"password123","confirmPassword":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "setup request body is too large") {
		t.Fatalf("body=%q want setup body limit error", body)
	}
}

func TestHandleSetupRejectsOversizedFormPayload(t *testing.T) {
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
	oversizedEmailLocal := strings.Repeat("a", maxSetupRequestBodyBytes)
	form := "token=test-setup-token&email=" + oversizedEmailLocal + "%40example.com&name=Admin&password=password123&confirmPassword=password123"
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "setup request body is too large") {
		t.Fatalf("body=%q want setup body limit error", body)
	}
}

func TestSetupPageIncludesMCPSetupResultShell(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/setup?token=test-setup-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="setup-result"`,
		`data-copy-target="api-key"`,
		`id="mcp-config"`,
		`id="codex-command"`,
		`id="claude-command"`,
		`agen8.sessionToken`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("setup page missing %q", want)
		}
	}
}

func TestHandleSetupJSONIncludesMCPArtifacts(t *testing.T) {
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

	setupBody := `{"token":"test-setup-token","email":"admin@example.com","name":"Admin","password":"password123","confirmPassword":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "http://0.0.0.0:7777/setup", strings.NewReader(setupBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result struct {
		APIKey struct {
			Secret string `json:"secret"`
		} `json:"apiKey"`
		MCP struct {
			URL                string `json:"url"`
			CompatibilityURL   string `json:"compatibilityUrl"`
			Config             string `json:"config"`
			CodexCommand       string `json:"codexCommand"`
			ClaudeCommand      string `json:"claudeCommand"`
			CodexSkillCommand  string `json:"codexSkillCommand"`
			ClaudeSkillCommand string `json:"claudeSkillCommand"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	if result.APIKey.Secret == "" {
		t.Fatal("setup response missing api key secret")
	}
	if result.MCP.URL != "http://127.0.0.1:7777/mcp" {
		t.Fatalf("mcp url=%q want bearer-auth URL without query token", result.MCP.URL)
	}
	if !strings.HasPrefix(result.MCP.CompatibilityURL, "http://127.0.0.1:7777/mcp?token=ak_") {
		t.Fatalf("compatibility mcp url=%q want loopback URL with API key", result.MCP.CompatibilityURL)
	}
	for _, want := range []string{
		result.APIKey.Secret,
		`"mcpServers"`,
		`"bearer_token_env_var": "AGEN8_MCP_TOKEN"`,
		"codex mcp add agen8 --url",
		"--bearer-token-env-var AGEN8_MCP_TOKEN",
		"claude mcp add --transport http --scope user agen8",
		"--header",
		"agen8 skill install --harness codex",
		"agen8 skill install --harness claude-cli",
	} {
		joined := strings.Join([]string{
			result.MCP.URL,
			result.MCP.CompatibilityURL,
			result.MCP.Config,
			result.MCP.CodexCommand,
			result.MCP.ClaudeCommand,
			result.MCP.CodexSkillCommand,
			result.MCP.ClaudeSkillCommand,
		}, "\n")
		if !strings.Contains(joined, want) {
			t.Fatalf("mcp artifacts missing %q in %s", want, joined)
		}
	}
}

func TestSetupMCPArtifactsRejectHostHeaderPoisoning(t *testing.T) {
	handler := httpSetupHandler{httpAddr: "0.0.0.0:7777"}
	req := httptest.NewRequest(http.MethodPost, "http://attacker.example/setup", nil)

	result, err := handler.setupMCPArtifacts(req, "ak_secret")
	if err != nil {
		t.Fatalf("setupMCPArtifacts: %v", err)
	}
	joined := strings.Join([]string{
		result.URL,
		result.CompatibilityURL,
		result.Config,
		result.CodexCommand,
		result.ClaudeCommand,
	}, "\n")
	if strings.Contains(joined, "attacker.example") {
		t.Fatalf("setup artifacts trusted poisoned host: %s", joined)
	}
	if !strings.Contains(joined, "http://127.0.0.1:7777/mcp") {
		t.Fatalf("setup artifacts did not fall back to loopback daemon address: %s", joined)
	}
}

func TestSetupStatusRPCReturnsSetupURLWhileOpen(t *testing.T) {
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: t.TempDir()},
		SetupToken: "test setup token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := d.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"auth.setupStatus","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp rpc.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("rpc error=%+v", resp.Error)
	}
	var result struct {
		SetupOpen bool   `json:"setupOpen"`
		SetupURL  string `json:"setupUrl"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode setup status result: %v", err)
	}
	if !result.SetupOpen {
		t.Fatal("setup should be open")
	}
	if result.SetupURL != "/setup?token=test+setup+token" {
		t.Fatalf("setupUrl=%q", result.SetupURL)
	}
}

func TestSetupStatusRPCClosesAfterSetup(t *testing.T) {
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
	setupSessionForEventsTest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"auth.setupStatus","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp rpc.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
	var result struct {
		SetupOpen bool   `json:"setupOpen"`
		SetupURL  string `json:"setupUrl"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode setup status result: %v", err)
	}
	if result.SetupOpen || result.SetupURL != "" {
		t.Fatalf("result=%+v want closed without setup URL", result)
	}
}

func TestHandleSetupFormRevealsMCPArtifacts(t *testing.T) {
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

	form := "token=test-setup-token&email=admin%40example.com&name=Admin&password=password123&confirmPassword=password123"
	req := httptest.NewRequest(http.MethodPost, "http://0.0.0.0:7777/setup", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Setup complete",
		"ak_",
		"http://127.0.0.1:7777/mcp",
		"Compatibility query-token URL",
		"http://127.0.0.1:7777/mcp?token=ak_",
		".mcp.json",
		"codex mcp add agen8 --url",
		"--bearer-token-env-var AGEN8_MCP_TOKEN",
		"claude mcp add --transport http --scope user agen8",
		"--header",
		"agen8 skill install --harness codex",
		"agen8.sessionToken",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("setup form response missing %q", want)
		}
	}
}

func TestMCPAcceptsBearerAuthorizationToken(t *testing.T) {
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

	initializeBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(initializeBody))
	req.Header.Set("Authorization", "Bearer "+setupResult.APIKey.Secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mcp initialize status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestParseLoopbackDevWebURLAcceptsLocalTargets(t *testing.T) {
	tests := []string{
		"http://localhost:3000",
		"https://127.0.0.1:3000",
		"http://[::1]:3000",
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			parsed, err := parseLoopbackDevWebURL(tc)
			if err != nil {
				t.Fatalf("parseLoopbackDevWebURL(%q): %v", tc, err)
			}
			if parsed == nil || parsed.Host == "" {
				t.Fatalf("parseLoopbackDevWebURL(%q) returned empty host", tc)
			}
		})
	}
}

func TestParseLoopbackDevWebURLRejectsUnsafeValues(t *testing.T) {
	tests := []string{
		"",
		"http://",
		"https://example.com",
		"http://alice:secret@127.0.0.1:8080",
		"http://bob@localhost:8080",
		"ftp://127.0.0.1:8080",
		"http://192.168.0.1:8080",
		"http://10.0.0.1:8080",
		"http://%6c%6f%63%61%6c%68%6f%73%74:3000",
		"http://[::1%25EN0]:3000",
		"http://%e0%ae:3000",
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			if _, err := parseLoopbackDevWebURL(tc); err == nil {
				t.Fatalf("parseLoopbackDevWebURL(%q): want error", tc)
			}
		})
	}
}

func TestEnsureLoopbackHostnameRequiresLoopbackResolution(t *testing.T) {
	if err := ensureLoopbackHostname("localhost"); err != nil {
		t.Fatalf("ensureLoopbackHostname(localhost): %v", err)
	}
	if err := ensureLoopbackHostname("example.com"); err == nil {
		t.Fatalf("ensureLoopbackHostname(example.com) unexpectedly succeeded")
	}
}

func TestLoopbackAwareDialContextRejectsInvalidDialTarget(t *testing.T) {
	_, err := loopbackAwareDialContext()(context.Background(), "tcp", "localhost")
	if err == nil || !strings.Contains(err.Error(), "proxy target host is invalid") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestLoopbackAwareDialContextRejectsNonLoopbackDrift(t *testing.T) {
	oldLookup := lookupLoopbackHostIPs
	lookupLoopbackHostIPs = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("198.51.100.10")}, nil
	}
	t.Cleanup(func() {
		lookupLoopbackHostIPs = oldLookup
	})

	_, err := loopbackAwareDialContext()(context.Background(), "tcp", "dev.local:443")
	if err == nil {
		t.Fatal("loopbackAwareDialContext should reject non-loopback drift")
	}
	if !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("unexpected error=%v", err)
	}
}

func TestLoopbackAwareDialContextAllowsLoopbackHostValidationToProceed(t *testing.T) {
	oldLookup := lookupLoopbackHostIPs
	lookupLoopbackHostIPs = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() {
		lookupLoopbackHostIPs = oldLookup
	})

	_, err := loopbackAwareDialContext()(context.Background(), "tcp", "localhost:80")
	if err == nil {
		t.Fatalf("expected network error when dialing localhost without listener")
	}
	if strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("loopback host was blocked by loopback validation: %v", err)
	}
	if strings.Contains(err.Error(), "proxy target host is invalid") {
		t.Fatalf("loopback host parsing unexpectedly failed: %v", err)
	}
}

func TestWebHandlerFailsForUnsafeDevWebURL(t *testing.T) {
	t.Setenv(EnvDevWebURL, "http://example.com")

	d, err := New(Config{AppConfig: config.Config{DataDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = d.webHandler()
	if err == nil {
		t.Fatal("webHandler should fail with non-loopback web URL")
	}
}

func TestWebHandlerBuildsProxyForLocalDevWebURL(t *testing.T) {
	t.Setenv(EnvDevWebURL, "http://127.0.0.1:8080")

	d, err := New(Config{AppConfig: config.Config{DataDir: t.TempDir()}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	h, err := d.webHandler()
	if err != nil {
		t.Fatalf("webHandler: %v", err)
	}
	if h == nil {
		t.Fatal("webHandler returned nil handler")
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

func TestEventsStreamsPinChangesToMatchingProjectClients(t *testing.T) {
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

	sessionToken := setupSessionForEventsTest(t, handler)
	projectA := createProjectForLinkTokenTest(t, handler, sessionToken, t.TempDir())
	projectB := createProjectForLinkTokenTest(t, handler, sessionToken, t.TempDir())

	hubCtx, cancelHub := context.WithCancel(context.Background())
	t.Cleanup(cancelHub)
	go func() {
		if err := d.events.Run(hubCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("events hub: %v", err)
		}
	}()
	select {
	case <-d.events.Running():
	case <-time.After(2 * time.Second):
		t.Fatal("events hub did not become ready")
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	clientA1 := openEventsClient(t, server.URL, sessionToken, projectA)
	defer clientA1.res.Body.Close()
	clientA2 := openEventsClient(t, server.URL, sessionToken, projectA)
	defer clientA2.res.Body.Close()
	clientB := openEventsClient(t, server.URL, sessionToken, projectB)
	defer clientB.res.Body.Close()

	pinBody := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":9,"method":"pin.add","params":{"projectId":%q,"nodeRef":"task-live","nodeType":"task"}}`, projectA))
	req, err := http.NewRequest(http.MethodPost, server.URL+"/rpc", bytes.NewReader(pinBody))
	if err != nil {
		t.Fatalf("build pin request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	pinRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pin.add request: %v", err)
	}
	defer pinRes.Body.Close()
	if pinRes.StatusCode != http.StatusOK {
		t.Fatalf("pin.add status=%d", pinRes.StatusCode)
	}

	for name, stream := range map[string]*eventsTestClient{"project A client 1": clientA1, "project A client 2": clientA2} {
		event := readSSEData(t, stream.reader, 2*time.Second)
		if !strings.Contains(event, `"method":"event.append"`) {
			t.Fatalf("%s event=%s missing event.append", name, event)
		}
		if !strings.Contains(event, `"projectId":"`+projectA+`"`) {
			t.Fatalf("%s event=%s missing project A", name, event)
		}
		if !strings.Contains(event, `"type":"pin.added"`) {
			t.Fatalf("%s event=%s missing pin.added", name, event)
		}
	}
	if event := readSSEDataOptional(clientB.reader, 150*time.Millisecond); event != "" {
		t.Fatalf("project B unexpectedly received event: %s", event)
	}
}

func TestEventsRequireSessionCookie(t *testing.T) {
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: t.TempDir()},
		SetupToken: "test-setup-token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events?projectId=proj-1", nil)
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rec := httptest.NewRecorder()
	d.handleEvents(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEventsRejectsCrossOriginRequest(t *testing.T) {
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

	sessionToken := setupSessionForEventsTest(t, handler)
	projectID := createProjectForLinkTokenTest(t, handler, sessionToken, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events?projectId="+url.QueryEscape(projectID), nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken, Path: "/"})
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	d.handleEvents(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEventsRejectsProjectNotOwnedBySession(t *testing.T) {
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

	sessionToken := setupSessionForEventsTest(t, handler)
	_ = createProjectForLinkTokenTest(t, handler, sessionToken, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events?projectId=missing-project", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken, Path: "/"})
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rec := httptest.NewRecorder()
	d.handleEvents(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEventsRejectsOriginlessRequestWithoutReferer(t *testing.T) {
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

	sessionToken := setupSessionForEventsTest(t, handler)
	projectID := createProjectForLinkTokenTest(t, handler, sessionToken, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events?projectId="+url.QueryEscape(projectID), nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken, Path: "/"})
	rec := httptest.NewRecorder()
	d.handleEvents(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEventsAcceptsRefererMatchWhenOriginMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events", nil)
	req.Header.Set("Referer", "http://127.0.0.1:8080/setup")
	if !checkSameOriginRequest(req) {
		t.Fatalf("expected same-origin referer fallback to pass")
	}

	req.Header.Set("Origin", "https://127.0.0.1:8080")
	if checkSameOriginRequest(req) {
		t.Fatalf("expected origin-host mismatch to fail")
	}
}

func TestCheckSameOriginRequestWithRefererFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/events?projectId=project", nil)
	req.Header.Set("Referer", "https://127.0.0.1:8080/setup")
	if checkSameOriginRequest(req) {
		t.Fatalf("expected mismatched referer scheme to be rejected")
	}

	req.Header.Set("Referer", "http://127.0.0.1:8080/setup")
	if !checkSameOriginRequest(req) {
		t.Fatalf("expected referer fallback to accept same-origin request")
	}

	req.Header.Del("Origin")
	req.Header.Set("Referer", "http://evil.example/setup")
	if checkSameOriginRequest(req) {
		t.Fatalf("expected malicious referer to be rejected")
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
	if !strings.Contains(responseBody, "agen8") {
		t.Fatalf("mcp initialize body=%s want agen8 server info", responseBody)
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

func setupSessionForEventsTest(t *testing.T, handler http.Handler) string {
	t.Helper()
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
	return setupResult.Session.Token
}

type eventsTestClient struct {
	res    *http.Response
	reader *bufio.Reader
}

func openEventsClient(t *testing.T, baseURL, sessionToken, projectID string) *eventsTestClient {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/events?projectId="+url.QueryEscape(projectID), nil)
	if err != nil {
		t.Fatalf("build events request: %v", err)
	}
	if base, parseErr := url.Parse(baseURL); parseErr == nil {
		req.Header.Set("Origin", base.Scheme+"://"+base.Host)
	}
	req.Header.Set("Referer", baseURL+"/")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken, Path: "/"})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open events stream: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		t.Fatalf("events status=%d", res.StatusCode)
	}
	reader := bufio.NewReader(res.Body)
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, ": connected") {
		res.Body.Close()
		t.Fatalf("events initial line=%q err=%v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(line) != "" {
		res.Body.Close()
		t.Fatalf("events initial separator=%q err=%v", line, err)
	}
	return &eventsTestClient{res: res, reader: reader}
}

func readSSEData(t *testing.T, reader *bufio.Reader, timeout time.Duration) string {
	t.Helper()
	event := readSSEDataOptional(reader, timeout)
	if event == "" {
		t.Fatalf("timed out waiting for SSE data")
	}
	return event
}

func readSSEDataOptional(reader *bufio.Reader, timeout time.Duration) string {
	ch := make(chan string, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				ch <- ""
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data: ") {
				ch <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
	}()
	select {
	case event := <-ch:
		return event
	case <-time.After(timeout):
		return ""
	}
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
	return claimTaskBodyForSession(sessionID, "task-1")
}

// claimTaskBodyForSession is claimBodyForSession for a caller-chosen task id, so a wire
// test can claim a task it actually created instead of the placeholder id the
// resolveMCPSession-only callers use.
func claimTaskBodyForSession(sessionID, taskID string) []byte {
	return []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":9,
		"method":"tools/call",
		"params":{
			"name":"task",
			"arguments":{"action":"claim","task_id":%q,"session_id":%q}
		}
	}`, taskID, sessionID))
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
//   - Failure behavior is now unified: both invalid link tokens and invalid API keys
//     return the same collapsed error class so token state is not exposed through
//     branch-specific sentinels.
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

	// errClass names the error contract each token class must meet.
	type errClass int
	const (
		resolves           errClass = iota // success: nil error, usable member-less session
		storeMissCollapsed                 // auth failures collapse to the mcp store-miss error
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
		// Both link-token and API-key failures collapse to the same mcp store-miss error.
		{name: "revoked api key collapses to store-miss error", token: revokedKey.Token, want: storeMissCollapsed},
		{name: "expired api key collapses to store-miss error", token: expiredKey.Token, want: storeMissCollapsed},
		{name: "unminted api key collapses to store-miss error", token: "ak_this-key-does-not-exist", want: storeMissCollapsed},
		{name: "empty token collapses to store-miss error", token: "", want: storeMissCollapsed},
		{name: "revoked link token collapses to store-miss error", token: revokedLink.Token, want: storeMissCollapsed},
		{name: "expired link token collapses to store-miss error", token: expiredLink.Token, want: storeMissCollapsed},
		{name: "unminted link token collapses to store-miss error", token: "wlt_this-token-was-never-minted", want: storeMissCollapsed},
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
			if tc.want == storeMissCollapsed {
				assertLoudFailure(t, got, err)
				// authErr is discarded at daemon.go:281, so the specific auth sentinel must NOT
				// surface here - failure collapses to the generic mcp store-miss error.
				if errors.Is(err, auth.ErrTokenNotFound) || errors.Is(err, auth.ErrTokenExpired) {
					t.Fatalf("failure should collapse to the mcp store-miss error (authErr discarded at daemon.go:281), but a domain sentinel surfaced: %v", err)
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
	registerSessionMemberForTest(t, handler, apiKey, projectRoot, sessionID, "KR3 Happy Worker")

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

	// The content text is the decision tool's (lean) structured JSON: just the new
	// decision id. decision.log hard-requires an actor, so a server-generated dec- id
	// can ONLY exist if arguments.session_id resolved to the registered member AND that
	// member threaded through CallContext as the actor — a member-less call returns
	// "member_id is required" with no id. With a single registered member here, the id
	// alone proves the end-to-end actor binding. (Exact session->own-member resolution
	// under contention is covered by TestConcurrentSessionsResolveToOwnMember.)
	var logged struct {
		Decision struct {
			ID string `json:"id"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &logged); err != nil {
		t.Fatalf("decode decision content %q: %v", resp.Result.Content[0].Text, err)
	}
	if !strings.HasPrefix(logged.Decision.ID, "dec-") {
		t.Fatalf("decision id=%q want dec- prefix (only an actor-bound call produces one)", logged.Decision.ID)
	}
}

// getTaskBodyForSession builds a task.get tools/call carrying only arguments.session_id,
// so a wire test can read a task back as a specific registered member.
func getTaskBodyForSession(sessionID, taskID string) []byte {
	return []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":12,
		"method":"tools/call",
		"params":{
			"name":"task",
			"arguments":{"action":"get","task_id":%q,"session_id":%q}
		}
	}`, taskID, sessionID))
}

// wireToolResponse is the decoded JSON-RPC envelope of a tools/call over /mcp. It carries
// both the JSON-RPC error (a transport/protocol failure) and the tool result, including
// IsError - the flag mcpToolCallErrorResult sets when a tool returns an error. A loud tool
// refusal is IsError==true with the message in Content, NOT a JSON-RPC error.
type wireToolResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

// postMCPToolCall drives one tools/call body through the live /mcp pipeline under token
// and returns the decoded envelope. It asserts transport success (HTTP 200) only; the
// caller decides whether a tool success or a loud isError result is expected, because
// this suite asserts on both.
func postMCPToolCall(t *testing.T, handler http.Handler, token string, body []byte) wireToolResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp?token="+url.QueryEscape(token), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mcp tool status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp wireToolResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode mcp tool response: %v body=%s", err, rec.Body.String())
	}
	return resp
}

// wireToolSuccessText asserts resp is a clean tool success - no JSON-RPC error and no
// isError result - and returns its first content text, the tool's structured JSON.
func wireToolSuccessText(t *testing.T, resp wireToolResponse, label string) string {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("%s: unexpected rpc error: %s", label, resp.Error.Message)
	}
	if resp.Result.IsError {
		text := ""
		if len(resp.Result.Content) > 0 {
			text = resp.Result.Content[0].Text
		}
		t.Fatalf("%s: unexpected loud tool error: %s", label, text)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("%s: result missing content", label)
	}
	return resp.Result.Content[0].Text
}

// createTaskOverWireForSession drives task.create over the live /mcp pipeline as the
// member behind sessionID, assigns the new task to assigneeMemberID, and returns the
// server-generated task id. It asserts the new task starts pending and assigned to the
// requested member, so callers rely on a known claimable starting state.
func createTaskOverWireForSession(t *testing.T, handler http.Handler, token, sessionID, assigneeMemberID, title, description string) string {
	t.Helper()
	body := []byte(fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":11,
		"method":"tools/call",
		"params":{
			"name":"task",
			"arguments":{"action":"create","title":%q,"description":%q,"assignee_member_id":%q,"session_id":%q}
		}
	}`, title, description, assigneeMemberID, sessionID))
	text := wireToolSuccessText(t, postMCPToolCall(t, handler, token, body), "task.create")
	var created struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatalf("decode task.create content %q: %v", text, err)
	}
	if !strings.HasPrefix(created.Task.ID, "task-") {
		t.Fatalf("task.create id=%q want task- prefix", created.Task.ID)
	}
	if created.Task.Status != "pending" {
		t.Fatalf("new task status=%q want pending", created.Task.Status)
	}
	// The create ack is ultra-lean (id+status); assignee correctness is verified
	// via task.get in the tests that care (the binding gate reads claimedBy back).
	return created.Task.ID
}

// TestMCPWireBindingGateOnTaskClaim is KR2's end-to-end coverage of the binding gate that
// guards member-as-actor dispatch, driven through the live HTTP /mcp pipeline for
// task.claim. It pins the two halves of that gate side by side:
//
//   - Success: a task.claim carrying the in-band session_id of a registered member
//     resolves to that member and claims the task (pending -> active, claimedBy == the
//     member). The bound identity threads all the way into the task service.
//
//   - Loud failure (the multibinding-contention shape): over the SAME shared token -
//     bound to TWO registered members here, so the resolver genuinely has more than one
//     candidate - a task.claim whose session_id matches no registered member resolves
//     member-LESS. The task tool's actor gate (internal/mcp/tools/task/handler.go:
//     "task: registered member_id is required") then refuses the dispatch LOUDLY as an
//     isError result. It does NOT default to either registered member, and the task is
//     left untouched: still pending, still claimed by nobody. Only after the refusal does
//     the rightful member claim it, proving the task was claimable all along and the gate
//     refused on identity, not state.
//
// This is the property that makes the documented agen8-local multibinding bug
// (task-74b59b64) a loud, safe failure instead of a silent mis-attribution: when the
// actor member cannot be resolved for a session, dispatch stops rather than guessing. The
// trigger here is an unregistered session ref - a deterministic way to reach the same
// member-LESS state the production token-only ambiguity produces. So this test covers the
// GATE's response to that state, not the daemon's resolution ambiguity itself (that
// resolution contract is TestResolveMCPSessionTokenClasses, KR1).
//
// Residual risk this test does NOT close: its safety rests on resolution returning
// member-LESS under ambiguity, never a guessed member. If resolveMCPSession ever fell back
// to picking the sole/first member on a contended token, the gate would receive a (wrong)
// actor id and this member-less path would sail right past the mis-attribution. KR1 is what
// pins resolution to stay member-less for an unknown ref; KR1 and KR2 only close the gap
// together.
//
// Scope: distinct from KR3 (TestMCPWireHappyPathForBoundMemberVerb), which drives a
// different verb (decision.log) and only the happy path, and from the
// resolveMCPSession-only concurrency tests, which never drive a verb through a tool
// handler.
func TestMCPWireBindingGateOnTaskClaim(t *testing.T) {
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

	// Setup yields a real admin API key. Both members register under it and every later
	// task verb carries it, so it is a genuinely shared token.
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

	// Two members register under the SAME api key + project. Two members on one token is
	// the multibinding shape: the resolver has more than one candidate, so a call it
	// cannot pin to a session must refuse rather than pick one.
	const claimantSession = "kr2-bound-claimant"
	const otherSession = "kr2-other-member"
	claimant := registerSessionMemberForTest(t, handler, apiKey, projectRoot, claimantSession, "KR2 Bound Claimant")
	other := registerSessionMemberForTest(t, handler, apiKey, projectRoot, otherSession, "KR2 Other Member")
	if claimant == other {
		t.Fatalf("two sessions collided onto one member id %q", claimant)
	}

	// A claimable task assigned to the claimant. Registration created the project from
	// project_root, so the resolved sessions share that bound project.
	taskID := createTaskOverWireForSession(t, handler, apiKey, claimantSession, claimant, "KR2 claimable task", "Task used to prove the binding gate on claim.")

	// Loud-failure half FIRST, while the task is still pending and unclaimed: a claim
	// carrying a session_id that matches no registered member resolves member-LESS, and
	// the actor gate refuses the dispatch.
	failResp := postMCPToolCall(t, handler, apiKey, claimTaskBodyForSession("kr2-session-never-registered", taskID))
	if failResp.Error != nil {
		t.Fatalf("member-less claim returned a JSON-RPC error %q; the gate must surface a tool isError result instead", failResp.Error.Message)
	}
	if !failResp.Result.IsError {
		t.Fatalf("member-less claim must be a loud isError result, got a success: %+v", failResp.Result)
	}
	if len(failResp.Result.Content) == 0 {
		t.Fatalf("member-less claim isError result missing content")
	}
	if msg := failResp.Result.Content[0].Text; !strings.Contains(msg, "registered member_id is required") {
		t.Fatalf("member-less claim error=%q want it to name the missing registered member_id", msg)
	}

	// No wrong-actor default: reading the task back (as the claimant) shows the failed
	// claim claimed it for NOBODY - still pending, claimedBy still empty. The gate did not
	// borrow the claimant's, the other member's, or any identity.
	afterFail := wireToolSuccessText(t, postMCPToolCall(t, handler, apiKey, getTaskBodyForSession(claimantSession, taskID)), "task.get after failed claim")
	var afterFailTask struct {
		Task struct {
			Status            string `json:"status"`
			ClaimedByMemberID string `json:"claimedByMemberId"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(afterFail), &afterFailTask); err != nil {
		t.Fatalf("decode task.get content %q: %v", afterFail, err)
	}
	if afterFailTask.Task.Status != "pending" {
		t.Fatalf("after failed claim status=%q want pending (the refused dispatch must not mutate the task)", afterFailTask.Task.Status)
	}
	if afterFailTask.Task.ClaimedByMemberID != "" {
		t.Fatalf("after failed claim claimedBy=%q want empty (no wrong-actor default)", afterFailTask.Task.ClaimedByMemberID)
	}

	// Success half: the rightful member's session claims the same task. pending -> active,
	// claimedBy == the claimant - the bound identity threads through to the task service.
	okText := wireToolSuccessText(t, postMCPToolCall(t, handler, apiKey, claimTaskBodyForSession(claimantSession, taskID)), "task.claim by bound member")
	var claimed struct {
		Task struct {
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(okText), &claimed); err != nil {
		t.Fatalf("decode task.claim content %q: %v", okText, err)
	}
	if claimed.Task.Status != "active" {
		t.Fatalf("claimed task status=%q want active", claimed.Task.Status)
	}
	// The claim ack is lean (status only). Read the task back to prove the bound
	// member threaded through as the actor: claimedBy == the claimant.
	afterClaim := wireToolSuccessText(t, postMCPToolCall(t, handler, apiKey, getTaskBodyForSession(claimantSession, taskID)), "task.get after successful claim")
	var afterClaimTask struct {
		Task struct {
			ClaimedByMemberID string `json:"claimedByMemberId"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(afterClaim), &afterClaimTask); err != nil {
		t.Fatalf("decode task.get content %q: %v", afterClaim, err)
	}
	if afterClaimTask.Task.ClaimedByMemberID != claimant {
		t.Fatalf("claimed task claimedBy=%q want %q (the bound member must thread through as the actor)", afterClaimTask.Task.ClaimedByMemberID, claimant)
	}
}
