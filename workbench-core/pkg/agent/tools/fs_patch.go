package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSPatchTool applies a unified diff patch to a file in the VFS.
type FSPatchTool struct{}

func (t *FSPatchTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "fs_patch",
			Description: "[DIRECT - no discovery needed] Apply a unified diff patch to a file.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to patch"},
					"text": map[string]any{"type": "string", "description": "Unified diff text"},
				},
				"required":             []any{"path", "text"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSPatchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path string `json:"path"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSPatch,
		Path: resolveVFSPath(payload.Path),
		Text: payload.Text,
	}, nil
}
