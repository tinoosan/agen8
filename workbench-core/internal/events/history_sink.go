package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/validate"
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
//	data/sessions/<sessionId>/history/history.jsonl
//
// The host owns history: it is append-only and should not be writable via VFS.
type HistorySink struct {
	// Store is the backing store for history.jsonl.
	//
	// This is the storage boundary; HistorySink does not perform direct filesystem IO.
	Store store.HistoryAppender

	// Model is the model identifier used for this run (for provenance).
	// Example: "openai/gpt-5-mini".
	Model string

	// Now returns the timestamp for new history lines.
	// If nil, time.Now is used.
	Now func() time.Time
}

func (s HistorySink) Emit(ctx context.Context, runID string, event Event) error {
	if !enabled(event.History) {
		return nil
	}
	if err := validate.NonEmpty("history sink: runID", runID); err != nil {
		return err
	}
	if s.Store == nil {
		return fmt.Errorf("history sink: Store is required")
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
	if err := s.Store.AppendLine(ctx, b); err != nil {
		return fmt.Errorf("history sink: append: %w", err)
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
