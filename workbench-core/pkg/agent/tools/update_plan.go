package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// UpdatePlanTool overwrites /plan/HEAD.md with a checklist.
type UpdatePlanTool struct{}

func (t *UpdatePlanTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "update_plan",
			Description: "[DIRECT] Overwrite /plan/HEAD.md with the checklist. Use this at the start of multi-step work AND after each meaningful step to keep progress current.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"plan": map[string]any{"type": "string", "description": "Checklist-style plan (markdown) to write to /plan/HEAD.md"},
				},
				"required":             []any{"plan"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *UpdatePlanTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/plan/HEAD.md",
		Text: payload.Plan,
	}, nil
}
