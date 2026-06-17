package member

import "testing"

func TestCanonicalHarnessKind(t *testing.T) {
	cases := map[string]string{
		// Claude family collapses to the canonical label.
		"claude":      HarnessClaudeCode,
		"claude-cli":  HarnessClaudeCode,
		"claude-code": HarnessClaudeCode,
		"Claude-Code": HarnessClaudeCode,
		"  claude  ":  HarnessClaudeCode,
		// Everything else passes through untouched.
		"codex":   "codex",
		"bridge":  "bridge",
		"unknown": "unknown",
		"":        "",
		// A future harness must not be erased to unknown.
		"cursor": "cursor",
	}
	for in, want := range cases {
		if got := CanonicalHarnessKind(in); got != want {
			t.Errorf("CanonicalHarnessKind(%q) = %q, want %q", in, got, want)
		}
	}
}
