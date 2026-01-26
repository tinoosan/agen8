package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// TraceEventsSinceTool streams trace events since a cursor.
type TraceEventsSinceTool struct{}

func (t *TraceEventsSinceTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "trace_events_since",
			Description: "[DIRECT] Stream trace events since a cursor.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cursor":   map[string]any{"type": []any{"string", "null"}, "description": "Trace cursor (from previous trace call)."},
					"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)."},
					"limit":    map[string]any{"type": intOrNull, "description": "Max number of events to return."},
				},
				"required":             []any{"cursor", "maxBytes", "limit"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TraceEventsSinceTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Cursor   json.RawMessage `json:"cursor"`
		MaxBytes *int            `json:"maxBytes"`
		Limit    *int            `json:"limit"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.since",
		Input:  args,
	}, nil
}

// TraceEventsLatestTool reads the latest trace events.
type TraceEventsLatestTool struct{}

func (t *TraceEventsLatestTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "trace_events_latest",
			Description: "[DIRECT] Read the latest trace events.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)."},
					"limit":    map[string]any{"type": intOrNull, "description": "Max number of events to return."},
				},
				"required":             []any{"maxBytes", "limit"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TraceEventsLatestTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		MaxBytes *int `json:"maxBytes"`
		Limit    *int `json:"limit"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.latest",
		Input:  args,
	}, nil
}

// TraceEventsSummaryTool summarizes trace events.
type TraceEventsSummaryTool struct{}

func (t *TraceEventsSummaryTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "trace_events_summary",
			Description: "[DIRECT] Summarize trace events.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cursor":       map[string]any{"type": []any{"string", "null"}, "description": "Trace cursor (from previous trace call)."},
					"maxBytes":     map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)."},
					"limit":        map[string]any{"type": intOrNull, "description": "Max number of events to return."},
					"includeTypes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Event types to include in summary."},
				},
				"required":             []any{"cursor", "maxBytes", "limit", "includeTypes"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TraceEventsSummaryTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Cursor       json.RawMessage `json:"cursor"`
		MaxBytes     *int            `json:"maxBytes"`
		Limit        *int            `json:"limit"`
		IncludeTypes []string        `json:"includeTypes"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.summary",
		Input:  args,
	}, nil
}
