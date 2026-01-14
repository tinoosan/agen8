package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HistorySink appends enriched, immutable history lines to history.jsonl.
//
// This sink is intentionally separate from store.AppendEvent:
//   - store.AppendEvent writes the run's event log and mirrors to /trace for agent polling
//   - HistorySink writes a "verifiable source of truth" record of raw interactions
//     between the user, agent, and environment
//
// On disk (run-scoped today):
//
//	data/runs/<runId>/history/history.jsonl
//
// The host owns history: it is append-only and should not be writable via VFS.
type HistorySink struct {
	// BaseDir is the OS directory where history.jsonl lives.
	// Example: data/runs/<runId>/history
	BaseDir string

	// Model is the model identifier used for this run (for provenance).
	// Example: "openai/gpt-5-mini".
	Model string

	// Now returns the timestamp for new history lines.
	// If nil, time.Now is used.
	Now func() time.Time
}

func (s HistorySink) Emit(_ context.Context, runID string, event Event) error {
	if !enabled(event.History) {
		return nil
	}
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("history sink: runID is required")
	}
	if strings.TrimSpace(s.BaseDir) == "" {
		return fmt.Errorf("history sink: BaseDir is required")
	}

	now := time.Now
	if s.Now != nil {
		now = s.Now
	}

	origin := strings.TrimSpace(event.Origin)
	if origin == "" {
		origin = inferOrigin(event.Type)
	}

	model := ""
	if origin == "agent" || strings.HasPrefix(event.Type, "llm.") {
		model = strings.TrimSpace(s.Model)
	}

	line := struct {
		ID        string            `json:"id"`
		Timestamp string            `json:"ts"`
		RunID     string            `json:"runId"`
		Origin    string            `json:"origin"`
		Kind      string            `json:"kind"`
		Message   string            `json:"message"`
		Model     string            `json:"model,omitempty"`
		Data      map[string]string `json:"data,omitempty"`
	}{
		ID:        uuid.NewString(),
		Timestamp: now().UTC().Format(time.RFC3339Nano),
		RunID:     runID,
		Origin:    origin,
		Kind:      event.Type,
		Message:   event.Message,
		Model:     model,
		Data:      event.Data,
	}

	b, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("history sink: marshal: %w", err)
	}
	b = append(b, '\n')

	if err := os.MkdirAll(s.BaseDir, 0755); err != nil {
		return fmt.Errorf("history sink: mkdir: %w", err)
	}
	p := filepath.Join(s.BaseDir, "history.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("history sink: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("history sink: write: %w", err)
	}
	return nil
}

func inferOrigin(kind string) string {
	switch {
	case kind == "user.message":
		return "user"
	case strings.HasPrefix(kind, "agent."):
		// agent.op.response is still "environment" (host).
		if kind == "agent.op.response" || kind == "agent.turn.complete" {
			return "env"
		}
		return "agent"
	case strings.HasPrefix(kind, "llm."):
		return "env"
	case strings.HasPrefix(kind, "memory."):
		return "env"
	case strings.HasPrefix(kind, "context."):
		return "env"
	case strings.HasPrefix(kind, "run."):
		return "env"
	default:
		return "env"
	}
}
