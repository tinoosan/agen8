package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSListTool lists directory contents in the VFS.
type FSListTool struct{}

func (t *FSListTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "fs_list",
			Description: "[DIRECT - no discovery needed] List directory contents at a VFS path. Common paths: /project (user's project), /scratch (your scratch), /plan (planning workspace), /skills (skill documents), /tools (for tool_run discovery only).",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "VFS path to list (e.g. /tools, /scratch)"},
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
