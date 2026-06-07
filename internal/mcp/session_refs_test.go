package mcp

import (
	"encoding/json"
	"testing"
)

// SessionRequestContextFromJSONRPCBody is the single point both transports use to
// pull a caller's conversation identity out of a JSON-RPC request: Codex via
// params._meta, Claude Code via arguments.session_id (stamped by its PreToolUse
// hook). These tests pin the precedence and the generalization so neither
// harness's path can silently regress.

// Codex self-identifies through params._meta. This is the canonical Codex read and
// must keep working untouched.
func TestSessionRequestContextReadsMetaForCodex(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"method": "tools/call",
		"params": {
			"name": "task",
			"_meta": {"sessionId": "codex-sess", "threadId": "codex-thread", "turnId": "codex-turn"},
			"arguments": {"action": "claim", "task_id": "t1"}
		}
	}`)
	got := SessionRequestContextFromJSONRPCBody(body)
	if got.SessionID != "codex-sess" {
		t.Fatalf("SessionID=%q want codex-sess", got.SessionID)
	}
	if got.ThreadID != "codex-thread" {
		t.Fatalf("ThreadID=%q want codex-thread", got.ThreadID)
	}
	if got.TurnID != "codex-turn" {
		t.Fatalf("TurnID=%q want codex-turn", got.TurnID)
	}
}

// Codex's wrapper nests the refs under _meta["x-codex-turn-metadata"]. This branch
// is Codex-specific and must remain byte-for-byte intact.
func TestSessionRequestContextReadsCodexTurnMetadata(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"method": "tools/call",
		"params": {
			"name": "task",
			"_meta": {"x-codex-turn-metadata": {"session_id": "nested-sess", "thread_id": "nested-thread", "turn_id": "nested-turn"}},
			"arguments": {"action": "claim"}
		}
	}`)
	got := SessionRequestContextFromJSONRPCBody(body)
	if got.SessionID != "nested-sess" {
		t.Fatalf("SessionID=%q want nested-sess", got.SessionID)
	}
	if got.ThreadID != "nested-thread" {
		t.Fatalf("ThreadID=%q want nested-thread", got.ThreadID)
	}
	if got.TurnID != "nested-turn" {
		t.Fatalf("TurnID=%q want nested-turn", got.TurnID)
	}
}

// The generalization: arguments.session_id is now read for ANY tools/call, not
// only projecttool register. This is Claude Code's path - its hook cannot reach
// _meta, so it stamps session_id into the arguments of whatever verb it is
// calling (here task.claim). Before this change, a non-register call would have
// surfaced no session id and the caller would resolve member-less.
func TestSessionRequestContextReadsArgumentsForNonRegisterTool(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"method": "tools/call",
		"params": {
			"name": "task",
			"arguments": {"action": "claim", "task_id": "t1", "session_id": "claude-sess", "thread_id": "claude-thread"}
		}
	}`)
	got := SessionRequestContextFromJSONRPCBody(body)
	if got.SessionID != "claude-sess" {
		t.Fatalf("SessionID=%q want claude-sess", got.SessionID)
	}
	if got.ThreadID != "claude-thread" {
		t.Fatalf("ThreadID=%q want claude-thread", got.ThreadID)
	}
}

// When both channels carry an id, _meta wins. This preserves Codex precedence: a
// Codex caller that also happened to carry arguments.session_id would still be
// identified by its authoritative _meta value. The arguments read is strictly a
// fallback for callers (Claude) that supply nothing in _meta.
func TestSessionRequestContextMetaWinsOverArguments(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"method": "tools/call",
		"params": {
			"name": "task",
			"_meta": {"sessionId": "meta-sess"},
			"arguments": {"action": "claim", "session_id": "args-sess"}
		}
	}`)
	got := SessionRequestContextFromJSONRPCBody(body)
	if got.SessionID != "meta-sess" {
		t.Fatalf("SessionID=%q want meta-sess (meta must outrank arguments)", got.SessionID)
	}
}

// The arguments read is gated to tools/call. A different method (e.g. an
// initialize handshake) that happened to carry an arguments.session_id must not
// be mistaken for a tool invocation's identity.
func TestSessionRequestContextIgnoresArgumentsOutsideToolsCall(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"method": "initialize",
		"params": {"arguments": {"session_id": "should-be-ignored"}}
	}`)
	got := SessionRequestContextFromJSONRPCBody(body)
	if got.SessionID != "" {
		t.Fatalf("SessionID=%q want empty for non tools/call method", got.SessionID)
	}
}

// Malformed bodies must fail closed (empty refs), never panic. A blind resolver
// that paniced or returned garbage on bad input would be a denial-of-service and
// a correctness hole.
func TestSessionRequestContextOnInvalidBody(t *testing.T) {
	t.Parallel()
	got := SessionRequestContextFromJSONRPCBody([]byte("not json"))
	if got.SessionID != "" || got.ThreadID != "" || got.TurnID != "" {
		t.Fatalf("expected empty context for invalid body, got %+v", got)
	}
}

// HarnessFromJSONRPCBody is how the daemon auto-determines a member's harness at
// registration without the agent ever entering it. The signal is the same
// self-identification each client already stamps into the body. These tests pin
// each fingerprint and the honest "" (→ "unknown") fallback so no client gets
// silently mislabeled.
func TestHarnessFromJSONRPCBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      string
		nativeRef string
		want      string
	}{
		{
			name:      "bridge native ref wins",
			body:      `{"method":"tools/call","params":{"name":"project","arguments":{"action":"register","session_id":"s"}}}`,
			nativeRef: "bridge-deadbeef",
			want:      "bridge",
		},
		{
			name: "codex via x-codex-turn-metadata",
			body: `{"method":"tools/call","params":{"name":"project","_meta":{"x-codex-turn-metadata":{"session_id":"cs"}},"arguments":{"action":"register"}}}`,
			want: "codex",
		},
		{
			name: "codex via _meta camelCase refs",
			body: `{"method":"tools/call","params":{"name":"project","_meta":{"sessionId":"cs","threadId":"ct"},"arguments":{"action":"register"}}}`,
			want: "codex",
		},
		{
			name: "claude-code via arguments.session_id",
			body: `{"method":"tools/call","params":{"name":"project","arguments":{"action":"register","session_id":"claude-sess"}}}`,
			want: "claude-code",
		},
		{
			name: "codex _meta outranks claude arguments",
			body: `{"method":"tools/call","params":{"name":"project","_meta":{"sessionId":"cs"},"arguments":{"action":"register","session_id":"args-sess"}}}`,
			want: "codex",
		},
		{
			name: "no signal yields empty (caller records unknown)",
			body: `{"method":"tools/call","params":{"name":"project","arguments":{"action":"register","project_root":"/repo"}}}`,
			want: "",
		},
		{
			name: "arguments ignored outside tools/call",
			body: `{"method":"initialize","params":{"arguments":{"session_id":"x"}}}`,
			want: "",
		},
		{
			name: "malformed body fails closed to empty",
			body: `not json`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HarnessFromJSONRPCBody([]byte(tt.body), tt.nativeRef); got != tt.want {
				t.Fatalf("HarnessFromJSONRPCBody=%q want %q", got, tt.want)
			}
		})
	}
}

// stripAmbientSessionRefs removes the hook-injected refs before a tool handler
// decodes arguments. The handlers use DisallowUnknownFields with per-action
// whitelists, so any leftover session_id/thread_id on a verb that does not list
// them would make the whole call fail. These tests prove the refs are removed,
// every other field is preserved, and malformed/edge inputs fail open to the
// handler's own validation rather than being corrupted.
func TestStripAmbientSessionRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		in          string
		wantRemoved bool // session_id/thread_id must be absent from the result
		wantKeep    map[string]string
	}{
		{
			name:        "removes both refs and keeps the rest",
			in:          `{"action":"claim","task_id":"t1","session_id":"s","thread_id":"th"}`,
			wantRemoved: true,
			wantKeep:    map[string]string{"action": "claim", "task_id": "t1"},
		},
		{
			name:        "removes session_id only when thread absent",
			in:          `{"action":"claim","session_id":"s"}`,
			wantRemoved: true,
			wantKeep:    map[string]string{"action": "claim"},
		},
		{
			name:        "no refs present leaves arguments untouched",
			in:          `{"action":"claim","task_id":"t1"}`,
			wantRemoved: true, // nothing to remove; still must be absent
			wantKeep:    map[string]string{"action": "claim", "task_id": "t1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := stripAmbientSessionRefs(json.RawMessage(tt.in))
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(out, &obj); err != nil {
				t.Fatalf("result is not a JSON object: %v", err)
			}
			if tt.wantRemoved {
				if _, ok := obj["session_id"]; ok {
					t.Errorf("session_id must be stripped, got %s", out)
				}
				if _, ok := obj["thread_id"]; ok {
					t.Errorf("thread_id must be stripped, got %s", out)
				}
			}
			for k, want := range tt.wantKeep {
				var got string
				if err := json.Unmarshal(obj[k], &got); err != nil {
					t.Fatalf("field %q missing/undecodable in %s", k, out)
				}
				if got != want {
					t.Errorf("field %q=%q want %q", k, got, want)
				}
			}
		})
	}
}

// When there is nothing to strip, the helper returns the original slice unchanged
// - no needless re-marshal that would reorder keys for the common Codex case
// (Codex never carries arguments.session_id, so its calls hit this fast path).
func TestStripAmbientSessionRefsReturnsOriginalWhenNoRefs(t *testing.T) {
	t.Parallel()
	in := json.RawMessage(`{"action":"claim","task_id":"t1"}`)
	out := stripAmbientSessionRefs(in)
	if string(out) != string(in) {
		t.Fatalf("expected byte-identical passthrough, got %s want %s", out, in)
	}
}

// Non-object and empty arguments fail open: the helper returns them unchanged so
// the handler's own validation (json.Valid happens upstream; handlers reject
// non-objects) stays the single source of truth. The helper must never turn a
// valid-but-unexpected shape into something else.
func TestStripAmbientSessionRefsFailsOpen(t *testing.T) {
	t.Parallel()
	for _, in := range []string{``, `[]`, `[1,2,3]`, `"scalar"`, `not json`} {
		out := stripAmbientSessionRefs(json.RawMessage(in))
		if string(out) != in {
			t.Errorf("input %q must pass through unchanged, got %q", in, string(out))
		}
	}
}
