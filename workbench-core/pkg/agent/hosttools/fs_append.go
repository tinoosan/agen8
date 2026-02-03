package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSAppendTool appends text to a file in the VFS.
type FSAppendTool struct{}

func (t *FSAppendTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "fs.append",
			Description: "[DIRECT - no discovery needed] Append text to a file at a VFS path.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to append to"},
					"text": map[string]any{"type": "string", "description": "Text to append"},
				},
				"required":             []any{"path", "text"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSAppendTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path string `json:"path"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSAppend,
		Path: resolveVFSPath(payload.Path),
		Text: payload.Text,
	}, nil
}
