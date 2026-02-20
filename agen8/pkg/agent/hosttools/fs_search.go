package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSSearchTool searches a VFS mount using keyword/regex text search.
type FSSearchTool struct{}

func (t *FSSearchTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_search",
		"[DIRECT] Search files under a VFS path using keyword/regex text search. Prefer this over reading whole memory files.",
		map[string]any{
			"path":  map[string]any{"type": "string", "description": "VFS path to search (e.g. /memory)"},
			"query": map[string]any{"type": "string", "description": "Search query (keyword or regex)"},
			"limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
		},
		[]any{"path", "query", "limit"},
	)
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
