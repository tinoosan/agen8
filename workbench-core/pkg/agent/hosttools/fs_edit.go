package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSEditTool applies structured edits to a file in the VFS.
type FSEditTool struct{}

func (t *FSEditTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "fs_edit",
			Description: "[DIRECT - no discovery needed] Apply find-replace edits to a file. Each edit has old (exact match), new (replacement), occurrence (1-based).",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to edit"},
					"edits": map[string]any{
						"type":        "array",
						"description": "Edits to apply, in order.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"old":        map[string]any{"type": "string"},
								"new":        map[string]any{"type": "string"},
								"occurrence": map[string]any{"type": "integer"},
							},
							"required":             []any{"old", "new", "occurrence"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []any{"path", "edits"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSEditTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path  string `json:"path"`
		Edits []struct {
			Old        string `json:"old"`
			New        string `json:"new"`
			Occurrence int    `json:"occurrence"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	input, err := json.Marshal(map[string]any{"edits": payload.Edits})
	if err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:    types.HostOpFSEdit,
		Path:  resolveVFSPath(payload.Path),
		Input: input,
	}, nil
}
