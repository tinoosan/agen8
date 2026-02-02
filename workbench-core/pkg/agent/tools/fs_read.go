package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSReadTool reads a file from the VFS.
type FSReadTool struct{}

func (t *FSReadTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "fs_read",
			Description: "[DIRECT - no discovery needed] Read file contents from a VFS path (skills live under /skills/<skill_name>/SKILL.md).",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": "VFS path to read (e.g. /tools/<toolId>)"},
					"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)"},
				},
				"required":             []any{"path", "maxBytes"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSReadTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path     string `json:"path"`
		MaxBytes *int   `json:"maxBytes"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	maxBytes := 1024 * 1024
	if payload.MaxBytes != nil {
		maxBytes = *payload.MaxBytes
	}
	return types.HostOpRequest{
		Op:       types.HostOpFSRead,
		Path:     resolveVFSPath(payload.Path),
		MaxBytes: maxBytes,
	}, nil
}
