package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubBinder struct {
	result BindResult
	err    error
	input  HookInput
	calls  int
}

func (s *stubBinder) Bind(_ context.Context, input HookInput) (BindResult, error) {
	s.calls++
	s.input = input
	return s.result, s.err
}

func runHook(t *testing.T, binder HookBinder, stdin string) (hookOutput, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	RunHook(context.Background(), binder, strings.NewReader(stdin), &out, &errOut)
	var parsed hookOutput
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("hook output is not valid JSON %q: %v", out.String(), err)
	}
	return parsed, out.String(), errOut.String()
}

func TestRunHook_EmptySessionEmitsNoop(t *testing.T) {
	binder := &stubBinder{}
	parsed, raw, _ := runHook(t, binder, `{"hook_event_name":"SessionStart"}`)
	if strings.TrimSpace(raw) != "{}" {
		t.Fatalf("expected no-op {} for empty session, got %q", raw)
	}
	if binder.calls != 0 {
		t.Fatalf("binder called %d times", binder.calls)
	}
	if parsed.HookSpecificOutput != nil || parsed.SystemMessage != "" {
		t.Fatalf("expected empty hook output, got %#v", parsed)
	}
}

func TestRunHook_BindsAndInjectsContext(t *testing.T) {
	binder := &stubBinder{result: BindResult{MemberID: "member-claude", SpaceID: "space-1"}}
	parsed, _, _ := runHook(t, binder, `{"session_id":"sess-1","hook_event_name":"SessionStart","cwd":"/repo"}`)
	if binder.calls != 1 {
		t.Fatalf("binder called %d times", binder.calls)
	}
	if binder.input.SessionID != "sess-1" || binder.input.CWD != "/repo" {
		t.Fatalf("hook input not passed through: %#v", binder.input)
	}
	if parsed.HookSpecificOutput == nil {
		t.Fatalf("expected hookSpecificOutput, got %#v", parsed)
	}
	if parsed.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("event name not echoed: %q", parsed.HookSpecificOutput.HookEventName)
	}
	if !strings.Contains(parsed.HookSpecificOutput.AdditionalContext, "member-claude") {
		t.Fatalf("member missing from context: %q", parsed.HookSpecificOutput.AdditionalContext)
	}
}

func TestRunHook_BindErrorEmitsNoopAndLogs(t *testing.T) {
	binder := &stubBinder{err: errors.New("boom")}
	_, raw, errOut := runHook(t, binder, `{"session_id":"sess-1","hook_event_name":"SessionStart"}`)
	if strings.TrimSpace(raw) != "{}" {
		t.Fatalf("expected no-op {} on bind error, got %q", raw)
	}
	if !strings.Contains(errOut, "boom") {
		t.Fatalf("expected bind error logged to errOut, got %q", errOut)
	}
}

func TestAgen8URLFromMCPConfig(t *testing.T) {
	raw := []byte(`{"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:7777/mcp?token=abc"}}}`)
	got, ok := agen8URLFromMCPConfig(raw)
	if !ok || got != "http://127.0.0.1:7777/mcp?token=abc" {
		t.Fatalf("url=%q ok=%v", got, ok)
	}
}

func TestDiscoverAgen8MCPURLWalksParents(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(`{"mcpServers":{"agen8":{"url":"http://127.0.0.1:7777/mcp?token=abc"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := discoverAgen8MCPURL(HookInput{CWD: nested})
	if err != nil {
		t.Fatalf("discoverAgen8MCPURL: %v", err)
	}
	if got != "http://127.0.0.1:7777/mcp?token=abc" {
		t.Fatalf("url=%q", got)
	}
}

func TestMCPHookBinderRegistersClaudeSession(t *testing.T) {
	var sawRegister bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") || !strings.Contains(got, "text/event-stream") {
			t.Fatalf("Accept header=%q", got)
		}
		var req struct {
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "protocol-1")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"init","result":{"capabilities":{}}}`))
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") != "protocol-1" {
				t.Fatalf("initialized missing protocol session header")
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			if r.Header.Get("Mcp-Session-Id") != "protocol-1" {
				t.Fatalf("tools/call missing protocol session header")
			}
			if req.Params.Name != "space" {
				t.Fatalf("tool=%q", req.Params.Name)
			}
			if req.Params.Arguments["action"] != "register" ||
				req.Params.Arguments["harness_kind"] != "claude-cli" ||
				req.Params.Arguments["session_id"] != "claude-session-1" {
				t.Fatalf("arguments=%#v", req.Params.Arguments)
			}
			sawRegister = true
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"call","result":{"structuredContent":{"memberId":"member-1","spaceId":"space-1","sessionId":"claude-session-1","nativeSessionRef":"claude-session-1"}}}`))
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}))
	defer server.Close()

	result, err := (MCPHookBinder{MCPURL: server.URL + "/mcp?token=abc", HTTPClient: server.Client()}).Bind(context.Background(), HookInput{
		SessionID: "claude-session-1",
		CWD:       "/repo",
	})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if !sawRegister {
		t.Fatalf("register was not called")
	}
	if result.MemberID != "member-1" || result.NativeSessionRef != "claude-session-1" {
		t.Fatalf("result=%#v", result)
	}
}
