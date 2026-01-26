package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSWriteTool writes a file in the VFS.
type FSWriteTool struct{}

func (t *FSWriteTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "fs_write",
			Description: "[DIRECT - no discovery needed] Write/create a file at a VFS path. Just call it with path and text.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to write"},
					"text": map[string]any{"type": "string", "description": "File contents to write"},
				},
				"required":             []any{"path", "text"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSWriteTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path string `json:"path"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: resolveVFSPath(payload.Path),
		Text: payload.Text,
	}, nil
}
