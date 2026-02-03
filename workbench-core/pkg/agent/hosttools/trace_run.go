package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// TraceRunTool runs a trace action (e.g. events.latest).
type TraceRunTool struct{}

func (t *TraceRunTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "trace_run",
			Description: "[DIRECT] Run a trace action (events.latest, events.since, events.summary).",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Trace action to run.",
						"enum":        []any{"events.latest", "events.since", "events.summary"},
					},
					"input": map[string]any{
						"type":                 "object",
						"description":          "Trace action input (varies by action). For events.latest this can be an empty object.",
						"properties":           map[string]any{},
						"additionalProperties": true,
					},
				},
				"required":             []any{"action", "input"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TraceRunTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Action string          `json:"action"`
		Input  json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action == "" {
		return types.HostOpRequest{}, fmt.Errorf("action is required")
	}
	if payload.Input == nil {
		payload.Input = json.RawMessage(`{}`)
	}
	return types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: action,
		Input:  payload.Input,
	}, nil
}
