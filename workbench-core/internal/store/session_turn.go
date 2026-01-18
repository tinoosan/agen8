package store

import (
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
)

const (
	// MaxSessionGoalChars bounds CurrentGoal to keep session.json small and prompt injection cheap.
	MaxSessionGoalChars = 500
	// MaxSessionSummaryBytes bounds Summary so resume stays token-efficient.
	MaxSessionSummaryBytes = 8 * 1024
)

// RecordTurnInSession updates session-level state after completing one user turn.
//
// This is host policy (not provenance):
//   - /history remains the source of truth
//   - Session.CurrentGoal/Plan/Summary are a compact "resume state"
//
// The intent is to prevent "agent amnesia" when resuming a session by giving the host
// a small, bounded snapshot to inject into the system prompt.
func RecordTurnInSession(cfg config.Config, sessionID, runID, userText, agentFinal string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Session{}, err
	}
	if err := validate.NonEmpty("runId", runID); err != nil {
		return types.Session{}, err
	}

	s, err := LoadSession(cfg, sessionID)
	if err != nil {
		return types.Session{}, err
	}

	userText = strings.TrimSpace(userText)
	if userText != "" {
		s.CurrentGoal = clampString(userText, MaxSessionGoalChars)
	}

	now := time.Now().UTC()
	s.UpdatedAt = &now
	s.CurrentRunID = runID

	// Ensure run appears in the index for navigability.
	seen := false
	for _, existing := range s.Runs {
		if existing == runID {
			seen = true
			break
		}
	}
	if !seen {
		s.Runs = append(s.Runs, runID)
	}

	// Append a compact summary line (most recent last).
	agentFinal = strings.TrimSpace(agentFinal)
	line := now.Format(time.RFC3339Nano) + " run=" + runID
	if userText != "" {
		line += " user=" + oneLine(clampString(userText, 200))
	}
	if agentFinal != "" {
		line += " agent=" + oneLine(clampString(agentFinal, 200))
	}
	line += "\n"

	s.Summary = appendAndCapBytes(s.Summary, line, MaxSessionSummaryBytes)

	return s, SaveSession(cfg, s)
}

func clampString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func appendAndCapBytes(existing, appendLine string, maxBytes int) string {
	if maxBytes <= 0 {
		return existing + appendLine
	}
	out := existing + appendLine
	b := []byte(out)
	if len(b) <= maxBytes {
		return out
	}
	// Keep the tail so "most recent last" is preserved.
	tail := b[len(b)-maxBytes:]
	// Trim to a line boundary if possible.
	if idx := bytesIndexByte(tail, '\n'); idx >= 0 && idx < len(tail)-1 {
		tail = tail[idx+1:]
	}
	return string(tail)
}

func bytesIndexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}
