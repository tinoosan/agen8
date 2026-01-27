package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// ContentEvaluator evaluates whether an agent-written update.md should be committed.
//
// Both *agent.MemoryEvaluator and *agent.ProfileEvaluator satisfy this interface.
type ContentEvaluator interface {
	Evaluate(update string) (accepted bool, reason string, cleaned string)
}

// ContentCommitter is the minimal store contract needed to commit a cleaned update
// and append an audit record.
type ContentCommitter interface {
	AppendContent(ctx context.Context, text string) error
	AppendCommitLog(ctx context.Context, line types.MemoryCommitLine) error
}

type memoryCommitterAdapter struct{ pkgstore.MemoryCommitter }

func (a memoryCommitterAdapter) AppendContent(ctx context.Context, text string) error {
	return a.MemoryCommitter.AppendMemory(ctx, text)
}

type profileCommitterAdapter struct{ pkgstore.ProfileCommitter }

func (a profileCommitterAdapter) AppendContent(ctx context.Context, text string) error {
	return a.ProfileCommitter.AppendProfile(ctx, text)
}

// ContentUpdateProcessor ingests staged updates written by the agent (e.g.
// /memory/update.md, /profile/update.md) and commits them after evaluation.
//
// This intentionally mirrors the existing host-side pattern:
//   - read update.md
//   - evaluate
//   - emit evaluate event
//   - commit content (optional)
//   - emit commit event (optional)
//   - append audit log (best-effort)
//   - clear update.md
type ContentUpdateProcessor struct {
	FS        *vfs.FS
	Evaluator ContentEvaluator
	Store     ContentCommitter

	// Scope is a short identifier written into audit logs (e.g. "memory", "profile").
	Scope string
	// UpdatePath is the VFS path to read/clear (e.g. "/memory/update.md").
	UpdatePath string

	// Emit is used for evaluate/commit/audit events. If nil, events are skipped.
	Emit func(ctx context.Context, ev events.Event)

	// EmitAudit controls whether audit append failures/successes are emitted as events.
	// The current TUI behavior is to NOT emit audit events, so this defaults to false.
	EmitAudit bool
}

// ProcessUpdate ingests and commits a staged update if present.
//
// If UpdatePath cannot be read, it does nothing (mirrors prior behavior).
// If UpdatePath is readable, it always attempts to clear the file at the end.
func (p *ContentUpdateProcessor) ProcessUpdate(ctx context.Context, turn int, sessionID, runID, model string) error {
	if p == nil || p.FS == nil || strings.TrimSpace(p.UpdatePath) == "" {
		return nil
	}

	b, err := p.FS.Read(p.UpdatePath)
	if err != nil {
		return nil
	}
	// Always clear the staging file after any successful read.
	defer func() { _ = p.FS.Write(p.UpdatePath, []byte{}) }()

	updateRaw := string(b)
	trimmed := strings.TrimSpace(updateRaw)
	scope := strings.TrimSpace(p.Scope)

	emit := func(ev events.Event) {
		if p.Emit != nil {
			p.Emit(ctx, ev)
		}
	}

	if trimmed == "" {
		emit(events.Event{
			Type:    scope + ".evaluate",
			Message: "No " + scope + " update written",
			Data: map[string]string{
				"turn":     strconv.Itoa(turn),
				"accepted": "false",
				"reason":   "no_update",
				"bytes":    "0",
			},
		})
		return nil
	}

	hash := agent.SHA256Hex(trimmed)
	accepted, reason, cleaned := false, "evaluator_missing", ""
	if p.Evaluator != nil {
		accepted, reason, cleaned = p.Evaluator.Evaluate(updateRaw)
	}

	emit(events.Event{
		Type:    scope + ".evaluate",
		Message: "Evaluated " + scope + " update",
		Data: map[string]string{
			"turn":     strconv.Itoa(turn),
			"accepted": fmtBool(accepted),
			"reason":   reason,
			"bytes":    strconv.Itoa(len(trimmed)),
			"sha256":   hash[:12],
		},
	})

	if accepted && p.Store != nil {
		if err := p.Store.AppendContent(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
			emit(events.Event{
				Type:    scope + ".commit.error",
				Message: "Failed to commit " + scope + " update",
				Data:    map[string]string{"err": err.Error()},
				Store:   boolPtr(false),
			})
		} else {
			emit(events.Event{
				Type:    scope + ".commit",
				Message: "Committed " + scope + " update",
				Data: map[string]string{
					"turn":   strconv.Itoa(turn),
					"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
					"sha256": hash[:12],
				},
			})
		}
	}

	if p.Store != nil {
		auditErr := p.Store.AppendCommitLog(context.Background(), types.MemoryCommitLine{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Scope:     scope,
			SessionID: sessionID,
			RunID:     runID,
			Model:     model,
			Turn:      turn,
			Accepted:  accepted,
			Reason:    reason,
			Bytes:     len(trimmed),
			SHA256:    hash,
		})
		if p.EmitAudit {
			if auditErr != nil {
				emit(events.Event{
					Type:    scope + ".audit.error",
					Message: "Failed to append " + scope + " audit log",
					Data: map[string]string{
						"turn": strconv.Itoa(turn),
						"err":  auditErr.Error(),
					},
					Store: boolPtr(false),
				})
			} else {
				emit(events.Event{
					Type:    scope + ".audit.append",
					Message: "Appended " + scope + " audit log",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"sha256":   hash[:12],
					},
					Store: boolPtr(false),
				})
			}
		}
	}

	return nil
}

// formatRunMemoryAppend produces the exact block appended to memory.md/profile.md when an update
// is accepted by the host.
func formatRunMemoryAppend(update string) string {
	update = strings.TrimSpace(update)
	if update == "" {
		return ""
	}
	return "\n\n—\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + update + "\n"
}
