// Package claudehook implements the Claude Code hook entrypoint that gives each
// Claude conversation its own identity inside agen8.
//
// The problem: several Claude Code conversations can share one agen8 token (for
// example a user-scoped API key or a wlt_ link token). Claude Code does not send
// its conversation id to an MCP server - unlike Codex, which self-identifies via
// params._meta. So without help, every Claude conversation on a shared token
// looks identical to the daemon and they collide on a single member when calling
// member-as-actor verbs like task.claim.
//
// The fix: a PreToolUse hook. Claude Code runs this hook before every
// mcp__agen8__* tool call and hands it the conversation's session_id on stdin.
// The hook stamps that session_id into the tool call's arguments (tool_input).
// Claude Code forwards the modified arguments to the agen8 MCP server, where the
// daemon reads arguments.session_id to resolve THIS conversation to its own
// member (and then strips it before the tool's strict decoder runs). This is the
// Claude-side mirror of Codex's params._meta self-identification, routed through
// the same daemon resolver.
package claudehook

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	// agen8ToolPrefix identifies MCP tool calls routed to the agen8 server. Claude
	// Code names MCP tools mcp__<server>__<tool>; this server registers as "agen8".
	agen8ToolPrefix = "mcp__agen8__"
	// preToolUseEvent is the only hook event this entrypoint acts on. The same
	// `claude hook` command is wired for other events (e.g. SessionStart) too, so
	// we dispatch on the event name and pass everything else through untouched.
	preToolUseEvent = "PreToolUse"
)

// hookInput is the subset of the PreToolUse stdin payload we consume. Claude Code
// sends additional fields (transcript_path, cwd, permission_mode, ...); decoding
// only what we need keeps us forward-compatible with payload additions.
//
// tool_input is the tool's arguments object - for an MCP tool it is exactly the
// params.arguments that will be sent to the server. We decode it as a raw-message
// map so every original field is preserved byte-for-byte when we re-emit it.
type hookInput struct {
	HookEventName string                     `json:"hook_event_name"`
	SessionID     string                     `json:"session_id"`
	ToolName      string                     `json:"tool_name"`
	ToolInput     map[string]json.RawMessage `json:"tool_input"`
}

// Run reads a Claude Code hook payload from stdin and, for a PreToolUse event on
// an agen8 MCP tool, stamps the conversation's session_id into the tool input so
// the agen8 daemon can resolve THIS conversation to its own member.
//
// The modified input is emitted as hookSpecificOutput.updatedInput, which Claude
// Code forwards to the MCP server in place of the original arguments. No
// permissionDecision is set: the user's normal allow/deny flow is preserved, so
// credential-backed tools (e.g. http) are never silently auto-approved by the act
// of stamping identity.
//
// Failure philosophy (no fabricated identity): the hook must never invent a
// session id. If it cannot read one, it leaves the call unmodified and writes a
// diagnostic to stderr. A member-as-actor verb that genuinely needs a member then
// fails loudly at the tool layer - where the error belongs - instead of being
// silently misattributed to the wrong (or a fake) member here. Likewise, anything
// that is not a PreToolUse event for an agen8 tool passes straight through, so a
// mis-scoped matcher can never corrupt an unrelated tool's input.
func Run(stdin io.Reader, stdout, stderr io.Writer) error {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read hook stdin: %w", err)
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		// Unparseable payload: we cannot safely rewrite what we cannot read. Pass
		// the call through unchanged and surface the anomaly.
		fmt.Fprintf(stderr, "agen8 hook: could not parse hook input: %v\n", err)
		return nil
	}
	if strings.TrimSpace(in.HookEventName) != preToolUseEvent {
		return nil
	}
	if !strings.HasPrefix(strings.TrimSpace(in.ToolName), agen8ToolPrefix) {
		return nil
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		// Never fabricate identity. Surface the anomaly; leave the call unmodified.
		fmt.Fprintln(stderr, "agen8 hook: no session_id in PreToolUse payload; tool call left unmodified")
		return nil
	}

	// updatedInput REPLACES the original tool_input, so we must re-emit every
	// original field alongside the stamped session_id, not just the new field.
	updated := in.ToolInput
	if updated == nil {
		updated = map[string]json.RawMessage{}
	}
	encodedSessionID, err := json.Marshal(sessionID)
	if err != nil {
		return fmt.Errorf("encode session id: %w", err)
	}
	updated["session_id"] = encodedSessionID

	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName": preToolUseEvent,
			"updatedInput":  updated,
		},
	}
	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		return fmt.Errorf("write hook output: %w", err)
	}
	return nil
}
