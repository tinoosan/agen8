package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSListTool lists directory contents in the VFS.
type FSListTool struct{}

func (t *FSListTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "fs_list",
			Description: "[DIRECT] List directory contents at a VFS path. Common paths: /project (project files), /workspace (runtime files), /plan (planning files).",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to list (e.g. /project, /workspace)"},
				},
				"required":             []any{"path"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSListTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSList,
		Path: resolveVFSPath(payload.Path),
	}, nil
}
