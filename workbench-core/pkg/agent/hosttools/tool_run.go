package hosttools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// ToolRunTool executes a discovered tool by id + action.
type ToolRunTool struct{}

func (t *ToolRunTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "tool.run",
			Description: "[DISCOVERY] Run a discovered tool by id + action. Use fs.list/fs.read on /tools to discover manifests.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"toolId":   map[string]any{"type": "string", "description": "Tool id to run (e.g. \"builtin.shell\" or custom)."},
					"actionId": map[string]any{"type": "string", "description": "Action id to run (from manifest)."},
					"input": map[string]any{
						"type":                 "object",
						"description":          "Action input as JSON object.",
						"properties":           map[string]any{},
						"additionalProperties": true,
					},
					"timeoutMs": map[string]any{"type": intOrNull, "description": "Timeout override in ms (or null to use default)."},
				},
				"required":             []string{"toolId", "actionId", "input", "timeoutMs"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *ToolRunTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		ToolID    string          `json:"toolId"`
		ActionID  string          `json:"actionId"`
		Input     json.RawMessage `json:"input"`
		TimeoutMs *int            `json:"timeoutMs"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	if len(payload.Input) == 0 {
		payload.Input = json.RawMessage(`{}`)
	}
	var inputMap map[string]json.RawMessage
	if err := json.Unmarshal(payload.Input, &inputMap); err == nil {
		if cwdRaw, ok := inputMap["cwd"]; ok {
			var cwd string
			if err := json.Unmarshal(cwdRaw, &cwd); err == nil {
				inputMap["cwd"] = json.RawMessage(strconv.Quote(resolveVFSPath(cwd)))
				if updated, err := json.Marshal(inputMap); err == nil {
					payload.Input = updated
				}
			}
		}
	}
	timeout := 0
	if payload.TimeoutMs != nil {
		timeout = *payload.TimeoutMs
	}
	return types.HostOpRequest{
		Op:        types.HostOpToolRun,
		ToolID:    tools.ToolID(strings.TrimSpace(payload.ToolID)),
		ActionID:  strings.TrimSpace(payload.ActionID),
		Input:     payload.Input,
		TimeoutMs: timeout,
	}, nil
}
