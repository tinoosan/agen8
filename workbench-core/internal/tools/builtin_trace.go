package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinTraceManifest = []byte(`{
  "id": "builtin.trace",
  "version": "0.2.0",
  "kind": "builtin",
  "displayName": "Builtin Trace",
  "description": "Module-style access to run trace events using cursor-based APIs (no dynamic VFS paths).",
  "actions": [
    {
      "id": "events.since",
      "displayName": "Events Since",
      "description": "Return events from the given cursor forward, bounded by maxBytes/limit. Cursor is an opaque token; DiskTraceStore encodes it as a byte offset.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cursor": { "oneOf": [ { "type": "string" }, { "type": "number" } ] },
          "maxBytes": { "type": "integer" },
          "limit": { "type": "integer" }
        },
        "required": ["cursor"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "storeKind": { "type": "string" },
          "cursorBefore": { "type": "string" },
          "cursorAfter": { "type": "string" },
          "truncated": { "type": "boolean" },
          "bytesRead": { "type": "integer" },
          "eventsReturned": { "type": "integer" },
          "events": { "type": "array" }
        },
        "required": ["cursorBefore", "cursorAfter", "events"]
      }
    },
    {
      "id": "events.latest",
      "displayName": "Events Latest",
      "description": "Return the latest events (bounded by maxBytes/limit).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "maxBytes": { "type": "integer" },
          "limit": { "type": "integer" }
        }
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "storeKind": { "type": "string" },
          "cursorAfter": { "type": "string" },
          "truncated": { "type": "boolean" },
          "bytesRead": { "type": "integer" },
          "eventsReturned": { "type": "integer" },
          "events": { "type": "array" }
        },
        "required": ["events"]
      }
    },
    {
      "id": "events.summary",
      "displayName": "Events Summary",
      "description": "Return a deterministic, bounded summary of events since cursor (counts + recent lines).",
      "inputSchema": {
        "type": "object",
        "properties": {
          "cursor": { "oneOf": [ { "type": "string" }, { "type": "number" } ] },
          "maxBytes": { "type": "integer" },
          "limit": { "type": "integer" },
          "includeTypes": { "type": "array", "items": { "type": "string" } }
        },
        "required": ["cursor"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "storeKind": { "type": "string" },
          "cursorBefore": { "type": "string" },
          "cursorAfter": { "type": "string" },
          "truncated": { "type": "boolean" },
          "bytesRead": { "type": "integer" },
          "eventsReturned": { "type": "integer" },
          "typeCounts": { "type": "object" },
          "lastTimestamp": { "type": "string" },
          "summary": { "type": "string" }
        },
        "required": ["cursorBefore", "cursorAfter", "summary"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.trace"),
		Manifest: builtinTraceManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return BuiltinTraceInvoker{Store: cfg.TraceStore}
		},
	})
}

// BuiltinTraceInvoker implements builtin.trace.
//
// This tool is a "module as tool": it exposes callable actions that replace the
// older pattern of encoding dynamic queries into VFS paths (e.g. /log/events.since/<offset>).
//
// Unlike filesystem resources, module actions:
// - accept structured inputs
// - return structured outputs (events + cursor)
// - keep query semantics explicit and discoverable via the tool manifest
type BuiltinTraceInvoker struct {
	Store store.TraceStore
}

type traceSinceInput struct {
	Cursor   json.RawMessage `json:"cursor"`
	MaxBytes int             `json:"maxBytes,omitempty"`
	Limit    int             `json:"limit,omitempty"`
}

type traceSinceOutput struct {
	StoreKind      string             `json:"storeKind,omitempty"`
	CursorBefore   store.TraceCursor  `json:"cursorBefore"`
	CursorAfter    store.TraceCursor  `json:"cursorAfter"`
	Truncated      bool               `json:"truncated"`
	BytesRead      int                `json:"bytesRead"`
	EventsReturned int                `json:"eventsReturned"`
	Events         []store.TraceEvent `json:"events"`
}

type traceLatestInput struct {
	MaxBytes int `json:"maxBytes,omitempty"`
	Limit    int `json:"limit,omitempty"`
}

type traceLatestOutput struct {
	StoreKind      string             `json:"storeKind,omitempty"`
	CursorAfter    store.TraceCursor  `json:"cursorAfter"`
	Truncated      bool               `json:"truncated"`
	BytesRead      int                `json:"bytesRead"`
	EventsReturned int                `json:"eventsReturned"`
	Events         []store.TraceEvent `json:"events"`
}

func (t BuiltinTraceInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if t.Store == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "trace store not configured"}
	}

	switch req.ActionID {
	case "events.since":
		return t.eventsSince(ctx, req)
	case "events.latest":
		return t.eventsLatest(ctx, req)
	case "events.summary":
		return t.eventsSummary(ctx, req)
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: events.since, events.latest, events.summary)", req.ActionID)}
	}
}

func (t BuiltinTraceInvoker) eventsSince(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in traceSinceInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	cursor, err := parseCursor(in.Cursor)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	batch, err := t.Store.EventsSince(ctx, cursor, store.TraceSinceOptions{MaxBytes: in.MaxBytes, Limit: in.Limit})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	out := traceSinceOutput{
		StoreKind:      storeKind(t.Store),
		CursorBefore:   cursor,
		CursorAfter:    batch.CursorAfter,
		Truncated:      batch.Truncated,
		BytesRead:      batch.BytesRead,
		EventsReturned: len(batch.Events),
		Events:         batch.Events,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: b}, nil
}

func (t BuiltinTraceInvoker) eventsLatest(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in traceLatestInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	batch, err := t.Store.EventsLatest(ctx, store.TraceLatestOptions{MaxBytes: in.MaxBytes, Limit: in.Limit})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}
	out := traceLatestOutput{
		StoreKind:      storeKind(t.Store),
		CursorAfter:    batch.CursorAfter,
		Truncated:      batch.Truncated,
		BytesRead:      batch.BytesRead,
		EventsReturned: len(batch.Events),
		Events:         batch.Events,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: b}, nil
}

type traceSummaryInput struct {
	Cursor       json.RawMessage `json:"cursor"`
	MaxBytes     int             `json:"maxBytes,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	IncludeTypes []string        `json:"includeTypes,omitempty"`
}

type traceSummaryOutput struct {
	StoreKind      string            `json:"storeKind,omitempty"`
	CursorBefore   store.TraceCursor `json:"cursorBefore"`
	CursorAfter    store.TraceCursor `json:"cursorAfter"`
	Truncated      bool              `json:"truncated"`
	BytesRead      int               `json:"bytesRead"`
	EventsReturned int               `json:"eventsReturned"`
	TypeCounts     map[string]int    `json:"typeCounts,omitempty"`
	LastTimestamp  string            `json:"lastTimestamp,omitempty"`
	Summary        string            `json:"summary"`
}

func (t BuiltinTraceInvoker) eventsSummary(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	var in traceSummaryInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	cursor, err := parseCursor(in.Cursor)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}

	batch, err := t.Store.EventsSince(ctx, cursor, store.TraceSinceOptions{MaxBytes: in.MaxBytes, Limit: in.Limit})
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}

	counts := make(map[string]int)
	var lastTS string
	lines := make([]string, 0, len(batch.Events))
	include := make(map[string]bool, len(in.IncludeTypes))
	for _, t := range in.IncludeTypes {
		include[strings.TrimSpace(t)] = true
	}
	for _, ev := range batch.Events {
		if len(include) != 0 && !include[ev.Type] {
			continue
		}
		counts[ev.Type]++
		if ev.Timestamp != "" {
			lastTS = ev.Timestamp
		}
		if ev.Message != "" {
			lines = append(lines, ev.Type+": "+ev.Message)
		} else {
			lines = append(lines, ev.Type)
		}
	}

	summary := formatSummary(counts, lastTS, lines, 5)
	out := traceSummaryOutput{
		StoreKind:      storeKind(t.Store),
		CursorBefore:   cursor,
		CursorAfter:    batch.CursorAfter,
		Truncated:      batch.Truncated,
		BytesRead:      batch.BytesRead,
		EventsReturned: len(batch.Events),
		TypeCounts:     counts,
		LastTimestamp:  lastTS,
		Summary:        summary,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: b}, nil
}

func formatSummary(typeCounts map[string]int, lastTimestamp string, lines []string, keepLast int) string {
	var b strings.Builder
	if lastTimestamp != "" {
		b.WriteString("lastTimestamp: ")
		b.WriteString(lastTimestamp)
		b.WriteString("\n")
	}
	if len(typeCounts) != 0 {
		keys := make([]string, 0, len(typeCounts))
		for k := range typeCounts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("typeCounts:\n")
		for _, k := range keys {
			b.WriteString("- ")
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(strconv.Itoa(typeCounts[k]))
			b.WriteString("\n")
		}
	}

	if keepLast > 0 && len(lines) > keepLast {
		lines = lines[len(lines)-keepLast:]
	}
	if len(lines) != 0 {
		b.WriteString("recent:\n")
		for _, ln := range lines {
			b.WriteString("- ")
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func storeKind(s store.TraceStore) string {
	type kinder interface{ Kind() string }
	if k, ok := s.(kinder); ok {
		return k.Kind()
	}
	return ""
}

func parseCursor(raw json.RawMessage) (store.TraceCursor, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return "", fmt.Errorf("cursor is required")
	}
	// Try number first.
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		i, err := n.Int64()
		if err == nil && i >= 0 {
			return store.TraceCursorFromInt64(i), nil
		}
	}
	// Then string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", fmt.Errorf("cursor is required")
		}
		return store.TraceCursor(s), nil
	}
	return "", fmt.Errorf("cursor must be a string or number")
}
