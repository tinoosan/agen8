package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

var _ domain.SessionRuntime = Runtime{}

func TestStart_BuildsIdentityAndMCPArgs(t *testing.T) {
	rt := New()
	spec, err := rt.Start(domain.StartParams{
		SystemPrompt: "You are researcher",
		Model:        "anthropic/claude-sonnet-4.6",
		MCPServers:   []string{`{"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:7777/mcp?token=abc"}}}`},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if spec.Command != "claude" {
		t.Fatalf("command=%q want claude", spec.Command)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--system-prompt You are researcher") {
		t.Fatalf("missing identity arg: %v", spec.Args)
	}
	if !strings.Contains(args, "--output-format stream-json") {
		t.Fatalf("missing stream-json arg: %v", spec.Args)
	}
	if !strings.Contains(args, "--verbose") {
		t.Fatalf("missing --verbose required for stream-json: %v", spec.Args)
	}
	if !strings.Contains(args, "--model claude-sonnet-4-6") {
		t.Fatalf("missing normalized model arg: %v", spec.Args)
	}
	if strings.Contains(args, "anthropic/") {
		t.Fatalf("unexpected provider prefix in model arg: %v", spec.Args)
	}
	if strings.Contains(args, "4.6") {
		t.Fatalf("unexpected dotted version segment in model arg: %v", spec.Args)
	}
	if strings.Contains(args, "--config") {
		t.Fatalf("unexpected unsupported --config arg for claude cli: %v", spec.Args)
	}
	if !strings.Contains(args, `--mcp-config {"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:7777/mcp?token=abc"}}}`) {
		t.Fatalf("missing mcp arg: %v", spec.Args)
	}
}

func TestStartStreamingInput_UsesPrintAndRealtimeInput(t *testing.T) {
	rt := New()
	spec, err := rt.StartStreamingInput(domain.StartParams{
		SystemPrompt:    "You are researcher",
		Model:           "anthropic/claude-sonnet-4.6",
		ReasoningEffort: "high",
		SessionRef:      "session-abc",
		MCPServers:      []string{`{"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:7777/mcp?token=abc"}}}`},
	})
	if err != nil {
		t.Fatalf("StartStreamingInput: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	for _, want := range []string{
		"--print",
		"--input-format stream-json",
		"--output-format stream-json",
		"--verbose",
		"--session-id session-abc",
		"--model claude-sonnet-4-6",
		"--effort high",
		"--system-prompt You are researcher",
		`--mcp-config {"mcpServers":{"agen8":{"type":"http","url":"http://127.0.0.1:7777/mcp?token=abc"}}}`,
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("args=%q missing %q", args, want)
		}
	}
}

func TestStartStreamingInput_AddsPermissionPromptToolWhenApprovalsConfigured(t *testing.T) {
	rt := New()
	spec, err := rt.StartStreamingInput(domain.StartParams{
		PermissionMode: "claude-code/ask-permissions",
		ApprovalHandler: func(context.Context, domain.ApprovalRequest) (domain.ApprovalDecision, error) {
			return domain.ApprovalDecision{Decision: "approve"}, nil
		},
	})
	if err != nil {
		t.Fatalf("StartStreamingInput: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--permission-mode default") {
		t.Fatalf("missing default permission mode: %v", spec.Args)
	}
	if !strings.Contains(args, "--permission-prompt-tool "+claudePermissionPromptTool) {
		t.Fatalf("missing permission prompt tool: %v", spec.Args)
	}
}

func TestStartStreamingInput_DoesNotAddPermissionPromptToolForBypass(t *testing.T) {
	rt := New()
	spec, err := rt.StartStreamingInput(domain.StartParams{
		PermissionMode: "claude-code/bypass-permissions",
		ApprovalHandler: func(context.Context, domain.ApprovalRequest) (domain.ApprovalDecision, error) {
			return domain.ApprovalDecision{Decision: "approve"}, nil
		},
	})
	if err != nil {
		t.Fatalf("StartStreamingInput: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if strings.Contains(args, "--permission-prompt-tool") {
		t.Fatalf("unexpected permission prompt tool for bypass: %v", spec.Args)
	}
}

func TestStartStreamingInput_DoesNotAddPermissionPromptToolForDefaultBypass(t *testing.T) {
	rt := New()
	spec, err := rt.StartStreamingInput(domain.StartParams{
		AppServerURL: "http://127.0.0.1:7777",
		ApprovalHandler: func(context.Context, domain.ApprovalRequest) (domain.ApprovalDecision, error) {
			return domain.ApprovalDecision{Decision: "approve"}, nil
		},
	})
	if err != nil {
		t.Fatalf("StartStreamingInput: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--permission-mode bypassPermissions") {
		t.Fatalf("missing default bypass permission mode: %v", spec.Args)
	}
	if strings.Contains(args, "--permission-prompt-tool") {
		t.Fatalf("unexpected permission prompt tool for default bypass: %v", spec.Args)
	}
}

func TestStartStreamingInput_ContinueUsesResume(t *testing.T) {
	rt := New()
	spec, err := rt.StartStreamingInput(domain.StartParams{
		Continue:     true,
		SessionRef:   "session-abc",
		SystemPrompt: "You are researcher",
	})
	if err != nil {
		t.Fatalf("StartStreamingInput: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--resume session-abc") {
		t.Fatalf("missing resume arg: %v", spec.Args)
	}
	if strings.Contains(args, "--session-id") {
		t.Fatalf("unexpected session-id arg on resume: %v", spec.Args)
	}
	if strings.Contains(args, "--system-prompt") || strings.Contains(args, "--append-system-prompt") {
		t.Fatalf("resumed claude streaming turn must not re-send system prompt: %v", spec.Args)
	}
}

func TestStart_UsesSupportedEffortFlag(t *testing.T) {
	rt := New()
	for _, start := range []struct {
		name string
		run  func(domain.StartParams) (domain.StartSpec, error)
	}{
		{name: "one shot", run: rt.Start},
		{name: "streaming input", run: rt.StartStreamingInput},
	} {
		t.Run(start.name, func(t *testing.T) {
			spec, err := start.run(domain.StartParams{ReasoningEffort: "high"})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			args := strings.Join(spec.Args, " ")
			if strings.Contains(args, "--reasoning-effort") {
				t.Fatalf("claude cli does not support --reasoning-effort: %v", spec.Args)
			}
			if !strings.Contains(args, "--effort high") {
				t.Fatalf("missing supported --effort flag: %v", spec.Args)
			}
		})
	}
}

func TestNormalizeClaudeCLIModel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "provider prefixed dotted", in: "anthropic/claude-sonnet-4.6", want: "claude-sonnet-4-6"},
		{name: "dotted without provider", in: "claude-3.7-sonnet", want: "claude-3-7-sonnet"},
		{name: "already normalized", in: "claude-sonnet-4-6", want: "claude-sonnet-4-6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeClaudeCLIModel(tc.in); got != tc.want {
				t.Fatalf("normalizeClaudeCLIModel(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStart_SetsSessionRefForNewConversation(t *testing.T) {
	rt := New()
	spec, err := rt.Start(domain.StartParams{
		SessionRef: "session-abc",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--session-id session-abc") {
		t.Fatalf("missing session-id arg: %v", spec.Args)
	}
}

func TestStart_ContinueUsesResumeWhenSessionRefSet(t *testing.T) {
	rt := New()
	spec, err := rt.Start(domain.StartParams{
		Continue:     true,
		SessionRef:   "session-abc",
		SystemPrompt: "You are researcher",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--resume session-abc") {
		t.Fatalf("missing resume arg: %v", spec.Args)
	}
	if strings.Contains(args, "--system-prompt") || strings.Contains(args, "--append-system-prompt") {
		t.Fatalf("resumed claude turn must not re-send system prompt: %v", spec.Args)
	}
}

func TestStart_ContinueWithoutSessionDoesNotAppendIdentity(t *testing.T) {
	rt := New()
	spec, err := rt.Start(domain.StartParams{
		Continue:     true,
		SystemPrompt: "You are researcher",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "--continue") {
		t.Fatalf("missing continue arg: %v", spec.Args)
	}
	if strings.Contains(args, "--system-prompt") || strings.Contains(args, "--append-system-prompt") {
		t.Fatalf("continued claude turn must not re-send system prompt: %v", spec.Args)
	}
}

func TestParseEvents_MapsKnownTypes(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"message_start","turn_id":"t1","session_id":"s1"}`,
		`{"type":"content_block_delta","turn_id":"t1","delta":"hello"}`,
		`{"type":"tool_use","turn_id":"t1","id":"call-1","name":"task"}`,
		`{"type":"tool_result","turn_id":"t1","tool_call_id":"call-1","content":"ok"}`,
		`{"type":"message_stop","turn_id":"t1"}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("events len=%d want 5", len(events))
	}
	if events[1].Type != domain.EventText || events[1].Text != "hello" {
		t.Fatalf("text event mismatch: %+v", events[1])
	}
	if events[0].SessionRef != "s1" {
		t.Fatalf("session id=%q want s1", events[0].SessionRef)
	}
	if events[2].Type != domain.EventToolCall || events[2].ToolName != "task" {
		t.Fatalf("tool call mismatch: %+v", events[2])
	}
	if events[2].Data["status"] != "in_progress" {
		t.Fatalf("tool call data mismatch: %+v", events[2].Data)
	}
}

func TestParseEvents_NormalizesTopLevelWrappedAgen8ToolUse(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"tool_use","turn_id":"t1","id":"call-1","name":"tool","input":{"server":"agen8","tool":"space","arguments":{"action":"members"}}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventToolCall || events[0].ToolName != "agen8/space" {
		t.Fatalf("event=%+v", events[0])
	}
	if events[0].Data["status"] != "in_progress" || events[0].Data["input"] != `{"action":"members"}` {
		t.Fatalf("tool call data=%+v", events[0].Data)
	}
}

func TestParseEvents_FailsLoudOnUnknownType(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"unknown"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestParseEvents_IgnoresSystemType(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"rate_limit_event","session_id":"s1","rate_limits":{"five_hour":{"used":120}}}`,
		`{"type":"message_start","turn_id":"t1","session_id":"s1"}`,
		`{"type":"text","turn_id":"t1","text":"hello"}`,
		`{"type":"message_stop","turn_id":"t1","session_id":"s1"}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("events len=%d want 4", len(events))
	}
	if events[0].Type != domain.EventTurnStarted {
		t.Fatalf("events[0].Type=%q want %q", events[0].Type, domain.EventTurnStarted)
	}
	if events[1].SessionRef != "s1" {
		t.Fatalf("events[1].SessionRef=%q want s1", events[1].SessionRef)
	}
	if events[2].Type != domain.EventText || events[2].Text != "hello" {
		t.Fatalf("events[2]=%+v", events[2])
	}
	if events[3].Type != domain.EventTurnCompleted {
		t.Fatalf("events[3].Type=%q want %q", events[3].Type, domain.EventTurnCompleted)
	}
}

func TestParseEvents_MapsTopLevelAssistantUserAndResultTypes(t *testing.T) {
	rt := New()
	stream := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"session-1"}`,
		`{"type":"assistant","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"planning"},{"type":"tool_use","id":"call-1","name":"tool","input":{"x":1}}]}}`,
		`{"type":"user","message":{"id":"msg-2","role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"ok"}]}]}}`,
		`{"type":"result","subtype":"success","session_id":"session-1","usage":{"input_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":80,"output_tokens":15}}`,
	}, "\n"))
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("events len=%d want 5", len(events))
	}
	if events[0].Type != domain.EventTurnStarted || events[0].SessionRef != "session-1" {
		t.Fatalf("events[0]=%+v", events[0])
	}
	if events[1].Type != domain.EventText || events[1].Text != "planning" {
		t.Fatalf("events[1]=%+v", events[1])
	}
	if events[2].Type != domain.EventToolCall || events[2].ToolCallID != "call-1" || events[2].ToolName != "tool" {
		t.Fatalf("events[2]=%+v", events[2])
	}
	if events[3].Type != domain.EventToolResult || events[3].ToolCallID != "call-1" || events[3].Text != "ok" {
		t.Fatalf("events[3]=%+v", events[3])
	}
	if events[4].Type != domain.EventTurnCompleted || events[4].SessionRef != "session-1" {
		t.Fatalf("events[4]=%+v", events[4])
	}
	if events[4].Usage == nil {
		t.Fatalf("events[4] usage missing")
	}
	if events[4].Usage.InputTokens != 200 || events[4].Usage.OutputTokens != 15 || events[4].Usage.TotalTokens != 215 || events[4].Usage.CacheReadInputTokens != 80 {
		t.Fatalf("events[4] usage=%+v", events[4].Usage)
	}
}

func TestParseEvents_ResultSubtypeErrorMapsToTurnFailed(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"result","subtype":"error","session_id":"session-1","error":{"message":"failed"}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventTurnFailed || events[0].Error != "failed" {
		t.Fatalf("events[0]=%+v", events[0])
	}
}

func TestParseEvents_AssistantAPIErrorMapsToTurnFailed(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"assistant","session_id":"session-1","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"API Error: 400 messages.1.content.3: ` + "`thinking`" + ` blocks cannot be modified"}]}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventTurnFailed {
		t.Fatalf("events[0].Type=%q want %q", events[0].Type, domain.EventTurnFailed)
	}
	if !strings.Contains(events[0].Error, "API Error: 400") {
		t.Fatalf("events[0].Error=%q", events[0].Error)
	}
}

func TestParseEvents_AssistantToolUseRequiresIDAndName(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use"}]}}`))
	if err == nil || !strings.Contains(err.Error(), "tool_use id is required") {
		t.Fatalf("expected tool_use id error, got %v", err)
	}
}

func TestParseEvents_UserToolResultRequiresToolUseID(t *testing.T) {
	rt := New()
	_, err := rt.ParseEvents([]byte(`{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}`))
	if err == nil || !strings.Contains(err.Error(), "tool_result tool_use_id is required") {
		t.Fatalf("expected tool_result tool_use_id error, got %v", err)
	}
}

func TestParseEvents_NormalizesWrappedAgen8ToolUseToNamespacedTool(t *testing.T) {
	rt := New()
	stream := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call-1","name":"tool","input":{"server":"agen8","tool":"decision","arguments":{"action":"log","title":"test"}}}]}}`)
	events, err := rt.ParseEvents(stream)
	if err != nil {
		t.Fatalf("ParseEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len=%d want 1", len(events))
	}
	if events[0].Type != domain.EventToolCall {
		t.Fatalf("event type=%q want %q", events[0].Type, domain.EventToolCall)
	}
	if got := events[0].ToolName; got != "agen8/decision" {
		t.Fatalf("tool name=%q want agen8/decision", got)
	}
	if got := events[0].Data["input"]; got != `{"action":"log","title":"test"}` {
		t.Fatalf("input payload=%q", got)
	}
}

func TestWritePromptAndToolResult(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	if err := rt.WritePrompt(&out, domain.PromptInput{TurnID: "t1", Text: "hello"}); err != nil {
		t.Fatalf("WritePrompt: %v", err)
	}
	if !strings.Contains(out.String(), `"type":"prompt"`) {
		t.Fatalf("prompt payload mismatch: %q", out.String())
	}
	out.Reset()
	if err := rt.WriteToolResult(&out, domain.ToolResultInput{TurnID: "t1", ToolCallID: "call-1", Result: "ok"}); err != nil {
		t.Fatalf("WriteToolResult: %v", err)
	}
	if !strings.Contains(out.String(), `"tool_call_id":"call-1"`) {
		t.Fatalf("tool result payload mismatch: %q", out.String())
	}
}

func TestWriteStreamingPromptUsesClaudeSDKUserMessage(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	if err := rt.WriteStreamingPrompt(&out, domain.PromptInput{TurnID: "t1", Text: "hello", ReasoningEffort: "high"}); err != nil {
		t.Fatalf("WriteStreamingPrompt: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`"type":"user"`,
		`"message"`,
		`"role":"user"`,
		`"content"`,
		`"type":"text"`,
		`"text":"hello"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("streaming prompt payload=%q missing %q", got, want)
		}
	}
	if strings.Contains(got, "reasoning_effort") {
		t.Fatalf("streaming prompt payload must not include per-message reasoning_effort: %q", got)
	}
}

func TestWriteStreamingPromptIncludesAttachmentPathText(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	if err := rt.WriteStreamingPrompt(&out, domain.PromptInput{
		TurnID: "t1",
		Attachments: []domain.PromptAttachment{{
			ID:        "attachment-1",
			Name:      "screen.png",
			MediaType: "image/png",
			URI:       "/tmp/screen.png",
		}},
	}); err != nil {
		t.Fatalf("WriteStreamingPrompt: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `Attached image screen.png`) || !strings.Contains(got, `/tmp/screen.png`) {
		t.Fatalf("streaming prompt payload=%q missing attachment path", got)
	}
}

func TestExecuteSessionTurnRunsClaudeStreamAndPersistsSessionRef(t *testing.T) {
	rt := New()
	dir := t.TempDir()
	command := filepath.Join(dir, "fake-claude")
	script := `#!/bin/sh
input="$(cat)"
case "$input" in
  *"hello from chat"*) ;;
  *) echo "missing expected prompt: $input" >&2; exit 13 ;;
esac
printf '%s\n' '{"type":"system","subtype":"init","session_id":"session-abc"}'
printf '%s\n' '{"type":"assistant","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"working"},{"type":"tool_use","id":"call-1","name":"tool","input":{"server":"agen8","tool":"space","arguments":{"action":"members"}}}]}}'
printf '%s\n' '{"type":"user","message":{"id":"msg-2","role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"ok"}]}]}}'
printf '%s\n' '{"type":"result","subtype":"success","session_id":"session-abc","usage":{"input_tokens":2,"output_tokens":3}}'
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}

	var persisted []string
	var events []domain.Event
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		Command:      command,
		SystemPrompt: "You are Fred",
		Model:        "anthropic/claude-sonnet-4.6",
		MCPServers:   []string{`{"mcpServers":{"agen8":{"url":"http://127.0.0.1:7777/mcp?token=abc"}}}`},
		PersistSessionRef: func(sessionRef string) error {
			persisted = append(persisted, sessionRef)
			return nil
		},
	}, domain.SessionTurnInput{TurnID: "turn-1", Text: "hello from chat"}, func(ev domain.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	if len(persisted) == 0 || persisted[len(persisted)-1] != "session-abc" {
		t.Fatalf("persisted session refs=%v want session-abc", persisted)
	}
	if len(events) != 5 {
		t.Fatalf("events len=%d want 5: %+v", len(events), events)
	}
	if events[1].Type != domain.EventText || events[1].Text != "working" {
		t.Fatalf("text event=%+v", events[1])
	}
	if events[2].Type != domain.EventToolCall || events[2].ToolName != "agen8/space" || events[2].Data["input"] != `{"action":"members"}` {
		t.Fatalf("tool call event=%+v", events[2])
	}
	if events[3].Type != domain.EventToolResult || events[3].ToolCallID != "call-1" || events[3].Data["result"] != "ok" {
		t.Fatalf("tool result event=%+v", events[3])
	}
	if events[4].Type != domain.EventTurnCompleted || events[4].Usage == nil || events[4].Usage.TotalTokens != 5 {
		t.Fatalf("completion event=%+v", events[4])
	}
}

func TestExecuteSessionTurnUsesRemoteBridgeWebSocket(t *testing.T) {
	rt := New()
	var gotWorkdir string
	var gotArgs []string
	var gotPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.CloseNow()
		_, specReader, err := conn.Reader(r.Context())
		if err != nil {
			t.Errorf("read spec: %v", err)
			return
		}
		rawSpec, err := io.ReadAll(specReader)
		if err != nil {
			t.Errorf("read spec body: %v", err)
			return
		}
		var spec struct {
			Type    string   `json:"type"`
			Args    []string `json:"args"`
			Workdir string   `json:"workdir"`
		}
		if err := json.Unmarshal(rawSpec, &spec); err != nil {
			t.Errorf("parse spec: %v", err)
			return
		}
		if spec.Type != remoteBridgeStartMessageType {
			t.Errorf("spec type=%q want %q", spec.Type, remoteBridgeStartMessageType)
			return
		}
		gotWorkdir = spec.Workdir
		gotArgs = spec.Args
		_, reader, err := conn.Reader(r.Context())
		if err != nil {
			t.Errorf("read prompt: %v", err)
			return
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("read prompt body: %v", err)
			return
		}
		gotPrompt = string(data)
		for _, line := range []string{
			`{"type":"message_start","session_id":"remote-session"}`,
			`{"type":"content_block_delta","delta":"hello"}`,
			`{"type":"message_stop"}`,
		} {
			if err := conn.Write(r.Context(), websocket.MessageText, []byte(line)); err != nil {
				t.Errorf("write event: %v", err)
				return
			}
		}
	}))
	defer server.Close()

	var sessionRef string
	var events []domain.Event
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{
		AppServerURL:      "ws" + strings.TrimPrefix(server.URL, "http"),
		Workdir:           "/srv/project",
		Model:             "anthropic/claude-sonnet-4.6",
		MCPServers:        []string{"{\n  \"mcpServers\": {\n    \"agen8\": {\"type\":\"http\",\"url\":\"http://127.0.0.1:7777/mcp?token=abc\"}\n  }\n}"},
		PersistSessionRef: func(ref string) error { sessionRef = ref; return nil },
		ReasoningEffort:   "high",
		SystemPrompt:      "You are remote",
	}, domain.SessionTurnInput{TurnID: "turn-remote", Text: "hello"}, func(ev domain.Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	if gotWorkdir != "/srv/project" {
		t.Fatalf("workdir header=%q", gotWorkdir)
	}
	args := strings.Join(gotArgs, " ")
	for _, want := range []string{"--print", "--model claude-sonnet-4-6", "--mcp-config", "--permission-mode bypassPermissions"} {
		if !strings.Contains(args, want) {
			t.Fatalf("remote args=%q missing %q", args, want)
		}
	}
	if !strings.Contains(args, "{\n") {
		t.Fatalf("remote args did not preserve multiline MCP config: %q", args)
	}
	if !strings.Contains(gotPrompt, `"type":"user"`) || !strings.Contains(gotPrompt, "hello") {
		t.Fatalf("prompt=%q", gotPrompt)
	}
	if sessionRef != "remote-session" {
		t.Fatalf("sessionRef=%q", sessionRef)
	}
	if len(events) == 0 {
		t.Fatalf("expected events")
	}
}

func TestExecuteSessionTurnFailsWithoutTerminalEvent(t *testing.T) {
	rt := New()
	dir := t.TempDir()
	command := filepath.Join(dir, "fake-claude")
	script := `#!/bin/sh
cat >/dev/null
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	_, err := rt.ExecuteSessionTurn(context.Background(), domain.StartParams{Command: command}, domain.SessionTurnInput{Text: "hello"}, nil)
	if err == nil || !strings.Contains(err.Error(), "without terminal event") {
		t.Fatalf("expected missing terminal error, got %v", err)
	}
}

func TestExecuteSessionTurnSynthesizesCompletionAfterSuccessfulProgressEOF(t *testing.T) {
	rt := New()
	dir := t.TempDir()
	command := filepath.Join(dir, "fake-claude")
	script := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"type":"text","text":"partial"}'
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake command: %v", err)
	}
	var events []domain.Event
	_, err := rt.ExecuteSessionTurn(
		context.Background(),
		domain.StartParams{Command: command},
		domain.SessionTurnInput{TurnID: "turn-1", Text: "hello"},
		func(ev domain.Event) { events = append(events, ev) },
	)
	if err != nil {
		t.Fatalf("ExecuteSessionTurn: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%+v", events)
	}
	if events[0].Type != domain.EventText || events[0].Text != "partial" {
		t.Fatalf("events[0]=%+v", events[0])
	}
	if events[1].Type != domain.EventTurnCompleted || events[1].TurnID != "turn-1" {
		t.Fatalf("events[1]=%+v", events[1])
	}
}

func TestWriteStreamingPromptPreservesPromptWhitespace(t *testing.T) {
	rt := New()
	var out bytes.Buffer
	text := "first paragraph\n\nsecond paragraph"
	if err := rt.WriteStreamingPrompt(&out, domain.PromptInput{TurnID: "t1", Text: text}); err != nil {
		t.Fatalf("WriteStreamingPrompt: %v", err)
	}
	var payload struct {
		Message struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &payload); err != nil {
		t.Fatalf("unmarshal prompt payload: %v", err)
	}
	if got := payload.Message.Content[0].Text; got != text {
		t.Fatalf("prompt text=%q want %q", got, text)
	}
}
