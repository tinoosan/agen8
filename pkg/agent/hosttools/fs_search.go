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
		"[DIRECT] Search files under any VFS path using plain-text or regex matching. Use this to discover relevant files and narrow candidates with previews/metadata before fs_read.",
		map[string]any{
			"path":    map[string]any{"type": "string", "description": "VFS path to search (for example /workspace, /project, /memory, /knowledge)"},
			"query":   map[string]any{"type": "string", "description": "Plain-text substring search (case-insensitive)."},
			"pattern": map[string]any{"type": "string", "description": "Regex search pattern. Overrides query when provided."},
			"glob":    map[string]any{"type": "string", "description": "Optional include glob applied relative to the searched path."},
			"exclude": map[string]any{
				"description": "Optional exclude glob or glob list.",
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			"previewLines":    map[string]any{"type": "integer", "description": "Context lines before and after the best match line."},
			"maxResults":      map[string]any{"type": "integer", "description": "Max results to return (default 5)."},
			"limit":           map[string]any{"type": "integer", "description": "Backward-compatible alias for maxResults."},
			"includeMetadata": map[string]any{"type": "boolean", "description": "Include sizeBytes and mtime metadata in each result."},
			"maxSizeBytes":    map[string]any{"type": "integer", "description": "Best-effort maximum file size to inspect."},
		},
		[]any{"path"},
	)
}

func (t *FSSearchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path            string          `json:"path"`
		Query           string          `json:"query"`
		Pattern         string          `json:"pattern"`
		Glob            string          `json:"glob"`
		Exclude         json.RawMessage `json:"exclude"`
		PreviewLines    int             `json:"previewLines"`
		MaxResults      int             `json:"maxResults"`
		Limit           int             `json:"limit"`
		IncludeMetadata bool            `json:"includeMetadata"`
		MaxSizeBytes    int64           `json:"maxSizeBytes"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	exclude, err := normalizeStringListArg(payload.Exclude)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	limit := payload.MaxResults
	if limit <= 0 {
		limit = payload.Limit
	}
	if limit <= 0 {
		limit = 5
	}
	return types.HostOpRequest{
		Op:              types.HostOpFSSearch,
		Path:            resolveVFSPath(payload.Path),
		Query:           payload.Query,
		Pattern:         payload.Pattern,
		Limit:           limit,
		Glob:            payload.Glob,
		Exclude:         exclude,
		PreviewLines:    payload.PreviewLines,
		IncludeMetadata: payload.IncludeMetadata,
		MaxSizeBytes:    payload.MaxSizeBytes,
	}, nil
}

func normalizeStringListArg(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(many))
	for _, item := range many {
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}
