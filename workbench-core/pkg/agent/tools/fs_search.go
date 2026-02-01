package agenttools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// FSSearchTool searches a VFS mount using a semantic index (e.g. /memory).
type FSSearchTool struct{}

func (t *FSSearchTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "fs_search",
			Description: "[DIRECT] Search a VFS mount using a semantic/indexed search (e.g. /memory). Prefer this over reading whole memory files.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "VFS path to search (e.g. /memory)"},
					"query": map[string]any{"type": "string", "description": "Search query"},
					"limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
				},
				"required":             []any{"path", "query", "limit"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *FSSearchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path  string `json:"path"`
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = 5
	}
	return types.HostOpRequest{
		Op:    types.HostOpFSSearch,
		Path:  resolveVFSPath(payload.Path),
		Query: payload.Query,
		Limit: limit,
	}, nil
}
