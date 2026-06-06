package claudehook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// decodeUpdatedInput pulls hookSpecificOutput.updatedInput out of the hook's
// stdout so tests can assert on the rewritten arguments.
func decodeUpdatedInput(t *testing.T, stdout []byte) map[string]string {
	t.Helper()
	var out struct {
		HookSpecificOutput struct {
			HookEventName string            `json:"hookEventName"`
			UpdatedInput  map[string]string `json:"updatedInput"`
			// permissionDecision must be absent so the user's normal allow/deny flow
			// is preserved; we assert on its presence via the raw bytes below.
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout, &out); err != nil {
		t.Fatalf("decode hook stdout %q: %v", stdout, err)
	}
	if out.HookSpecificOutput.HookEventName != preToolUseEvent {
		t.Fatalf("hookEventName=%q want %q", out.HookSpecificOutput.HookEventName, preToolUseEvent)
	}
	return out.HookSpecificOutput.UpdatedInput
}

// The core behaviour: a PreToolUse call to an agen8 tool gets the conversation's
// session_id stamped into its arguments, and - because updatedInput REPLACES the
// original tool_input - every original argument is preserved alongside it.
func TestRunStampsSessionIDForAgen8Tool(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "claude-conversation-1",
		"tool_name": "mcp__agen8__task",
		"tool_input": {"action": "claim", "task_id": "t1"}
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := decodeUpdatedInput(t, stdout.Bytes())
	if got["session_id"] != "claude-conversation-1" {
		t.Errorf("session_id=%q want claude-conversation-1", got["session_id"])
	}
	if got["action"] != "claim" {
		t.Errorf("original action lost: %v", got)
	}
	if got["task_id"] != "t1" {
		t.Errorf("original task_id lost: %v", got)
	}
}

// The hook must NOT auto-approve agen8 tools: stamping identity is orthogonal to
// permission. Some agen8 tools (http) are credential-backed and must keep going
// through the user's normal allow/deny prompt, so permissionDecision must be
// absent from the output entirely.
func TestRunDoesNotSetPermissionDecision(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "sess",
		"tool_name": "mcp__agen8__http",
		"tool_input": {"action": "request"}
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bytes.Contains(stdout.Bytes(), []byte("permissionDecision")) {
		t.Fatalf("hook must not emit permissionDecision (would auto-approve credential-backed tools): %s", stdout.String())
	}
}

// A PreToolUse call whose tool_input is absent still produces a valid updatedInput
// carrying just the session_id - the hook synthesises an object rather than
// emitting null.
func TestRunStampsWhenToolInputMissing(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "sess-2",
		"tool_name": "mcp__agen8__project"
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := decodeUpdatedInput(t, stdout.Bytes())
	if got["session_id"] != "sess-2" {
		t.Errorf("session_id=%q want sess-2", got["session_id"])
	}
}

// Identity is never fabricated. With no session_id in the payload the hook leaves
// the call unmodified (no stdout) and surfaces the anomaly on stderr, so a
// member-as-actor verb fails loudly downstream rather than being misattributed.
func TestRunNeverFabricatesIdentity(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "",
		"tool_name": "mcp__agen8__task",
		"tool_input": {"action": "claim"}
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout (passthrough) when session_id missing, got %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no session_id") {
		t.Fatalf("expected stderr warning about missing session_id, got %q", stderr.String())
	}
}

// A mis-scoped matcher must never let the hook rewrite a non-agen8 tool's input.
// session_id has no meaning to (and could corrupt) e.g. a Bash command.
func TestRunIgnoresNonAgen8Tool(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "sess",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected passthrough for non-agen8 tool, got %s", stdout.String())
	}
}

// The same `claude hook` command is wired for other events (SessionStart). Those
// must pass through untouched - only PreToolUse stamps.
func TestRunIgnoresNonPreToolUseEvent(t *testing.T) {
	in := `{
		"hook_event_name": "SessionStart",
		"session_id": "sess",
		"tool_name": "mcp__agen8__task"
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected passthrough for non-PreToolUse event, got %s", stdout.String())
	}
}

// Unparseable input fails open (no stdout, the call proceeds unmodified) and is
// surfaced on stderr - we never block or corrupt a call we could not read.
func TestRunPassesThroughUnparseableInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader("not json"), &stdout, &stderr); err != nil {
		t.Fatalf("Run should not hard-error on bad input: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout for unparseable input, got %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "could not parse") {
		t.Fatalf("expected stderr parse warning, got %q", stderr.String())
	}
}

// Defence-in-depth: the stamped value is the exact session_id, not a fabricated
// or defaulted one. A value with surrounding whitespace is trimmed to the real id.
func TestRunTrimsSessionID(t *testing.T) {
	in := `{
		"hook_event_name": "PreToolUse",
		"session_id": "  spaced-sess  ",
		"tool_name": "mcp__agen8__task",
		"tool_input": {"action": "claim"}
	}`
	var stdout, stderr bytes.Buffer
	if err := Run(strings.NewReader(in), &stdout, &stderr); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := decodeUpdatedInput(t, stdout.Bytes())
	if got["session_id"] != "spaced-sess" {
		t.Errorf("session_id=%q want trimmed spaced-sess", got["session_id"])
	}
}
