package member

import "strings"

// Canonical harness kinds. The MCP session resolver (HarnessFromJSONRPCBody)
// produces these; downstream consumers — the web's "resume session" affordance
// and the harness leaderboard — key off them by exact string.
const (
	HarnessClaudeCode = "claude-code"
	HarnessCodex      = "codex"
	HarnessBridge     = "bridge"
	HarnessUnknown    = "unknown"
)

// CanonicalHarnessKind collapses the Claude family to one label. Older
// registrations and label drift produced "claude" / "claude-cli" before the
// code standardized on "claude-code" (see member_reconcile.go), so a member can
// carry any of them. Everything else — codex, bridge, unknown, and any future
// harness — passes through untouched, so this never invents or erases a kind it
// doesn't recognize.
func CanonicalHarnessKind(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "claude", "claude-cli", "claude-code", "claudecode":
		return HarnessClaudeCode
	default:
		return trimmed
	}
}
