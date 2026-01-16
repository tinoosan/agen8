package agent

import (
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
)

// SessionContextBlock renders a small, bounded session state block that the host can
// inject into the agent system prompt.
//
// This is intentionally not "provenance" (that lives in /history). It exists to prevent
// agents from feeling like they "forgot what happened" on resume:
//   - CurrentGoal says what we're working on now.
//   - Plan (optional) captures a stable plan mode output.
//   - Summary is a compact rolling recap (most recent last).
func SessionContextBlock(s types.Session) string {
	var b strings.Builder

	goal := strings.TrimSpace(s.CurrentGoal)
	plan := strings.TrimSpace(s.Plan)
	summary := strings.TrimSpace(s.Summary)

	if goal == "" && plan == "" && summary == "" {
		return ""
	}

	b.WriteString("## Session State\n\n")

	if s.SessionID != "" {
		b.WriteString("- sessionId: " + s.SessionID + "\n")
	}
	if s.CurrentRunID != "" {
		b.WriteString("- currentRunId: " + s.CurrentRunID + "\n")
	}
	if s.UpdatedAt != nil {
		b.WriteString("- updatedAt: " + s.UpdatedAt.UTC().Format(time.RFC3339Nano) + "\n")
	}
	b.WriteString("\n")

	if goal != "" {
		b.WriteString("### Current Goal\n\n")
		b.WriteString(goal + "\n\n")
	}
	if plan != "" {
		b.WriteString("### Plan\n\n")
		b.WriteString(plan + "\n\n")
	}
	if summary != "" {
		b.WriteString("### Summary (most recent last)\n\n")
		b.WriteString(summary + "\n")
	}

	return strings.TrimSpace(b.String())
}
