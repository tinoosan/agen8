package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

const defaultTraceMaxEvents = 500

// TraceMiddleware centralizes trace cursor/state tracking and read fallbacks.
// It provides a shared trace view for context constructor/updater.
type TraceMiddleware struct {
	Store store.TraceStore
	FS    *vfs.FS

	Cursor store.TraceCursor
	Events []types.Event

	MaxEvents int
}

func (m *TraceMiddleware) ensureCursor() store.TraceCursor {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(string(m.Cursor)) == "" {
		m.Cursor = store.TraceCursorFromInt64(0)
	}
	return m.Cursor
}

func (m *TraceMiddleware) maxEvents() int {
	if m == nil || m.MaxEvents <= 0 {
		return defaultTraceMaxEvents
	}
	return m.MaxEvents
}

func (m *TraceMiddleware) ApplyBatch(mode string, batch store.TraceBatch) []types.Event {
	if m == nil {
		return nil
	}
	newEvents := toTypesEvents(batch.Events)
	switch strings.TrimSpace(mode) {
	case "latest":
		m.Events = newEvents
	default:
		m.Events = append(m.Events, newEvents...)
	}
	maxEvents := m.maxEvents()
	if maxEvents > 0 && len(m.Events) > maxEvents {
		m.Events = m.Events[len(m.Events)-maxEvents:]
	}
	return m.Events
}

func (m *TraceMiddleware) ReadSince(ctx context.Context, opts store.TraceSinceOptions) (mode, source string, batch store.TraceBatch, cursorBefore, cursorAfter store.TraceCursor, err error) {
	if m == nil {
		return "since", "/log/events.jsonl", store.TraceBatch{}, "", "", fmt.Errorf("trace middleware not configured")
	}
	cursor := m.ensureCursor()
	cursorBefore = cursor
	mode = "since"
	source = "/log/events.jsonl"

	if m.Store != nil {
		b, readErr := m.Store.EventsSince(ctx, cursor, opts)
		m.Cursor = b.CursorAfter
		return mode, source, b, cursorBefore, b.CursorAfter, readErr
	}

	// Fallback: use the mounted trace resource's callable methods (still avoids dynamic paths).
	if m.FS != nil {
		_, r, _, resErr := m.FS.Resolve("/log")
		if resErr == nil {
			if tr, ok := r.(traceSinceReader); ok {
				offset, err := store.TraceCursorToInt64(cursor)
				if err != nil {
					return mode, source, store.TraceBatch{CursorAfter: cursor}, cursorBefore, cursor, fmt.Errorf("invalid trace cursor for trace resource fallback")
				}
				raw, next, readErr := tr.ReadEventsSince(offset)
				linesTotal, parsed, parseErrors, events := parseTypesEventJSONL(raw)
				batch = store.TraceBatch{
					Events:         toTraceEvents(events),
					CursorAfter:    store.TraceCursorFromInt64(next),
					BytesRead:      len(raw),
					LinesTotal:     linesTotal,
					Parsed:         parsed,
					ParseErrors:    parseErrors,
					Returned:       len(events),
					ReturnedCapped: false,
				}
				m.Cursor = batch.CursorAfter
				return mode, "/log/events.jsonl", batch, cursorBefore, batch.CursorAfter, readErr
			}
		}
	}

	return mode, source, store.TraceBatch{CursorAfter: cursor}, cursorBefore, cursor, fmt.Errorf("trace store not configured and trace resource does not support cursor reads")
}

func (m *TraceMiddleware) Summary(includeTypes []string, budgetBytes int) (summary string, selected, capped, excluded int, truncated bool) {
	if m == nil {
		return "", 0, 0, 0, false
	}
	return summarizeTrace(m.Events, includeTypes, budgetBytes)
}

type traceSinceReader interface {
	ReadEventsSince(offset int64) ([]byte, int64, error)
}
