package hosttools

import (
	"context"
	"encoding/json"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSArchiveCreateTool creates an archive from a VFS file or directory.
type FSArchiveCreateTool struct{}

func (t *FSArchiveCreateTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_archive_create",
		"[DIRECT] Create an archive from a VFS file or directory.",
		map[string]any{
			"path":            map[string]any{"type": "string", "description": "Absolute VFS source path (file or directory)."},
			"destination":     map[string]any{"type": "string", "description": "Absolute VFS destination archive path."},
			"format":          map[string]any{"type": "string", "enum": []string{"zip", "tar", "tar.gz"}},
			"exclude":         map[string]any{"type": []any{"array", "null"}, "items": map[string]any{"type": "string"}},
			"includeMetadata": map[string]any{"type": []any{"boolean", "null"}, "description": "Preserve timestamps when available. Default true."},
		},
		[]any{"path", "destination", "format"},
	)
}

func (t *FSArchiveCreateTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path            string   `json:"path"`
		Destination     string   `json:"destination"`
		Format          string   `json:"format"`
		Exclude         []string `json:"exclude"`
		IncludeMetadata *bool    `json:"includeMetadata"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	includeMetadata := true
	if payload.IncludeMetadata != nil {
		includeMetadata = *payload.IncludeMetadata
	}
	return types.HostOpRequest{
		Op:              types.HostOpFSArchiveCreate,
		Path:            resolveVFSPath(payload.Path),
		Destination:     resolveVFSPath(payload.Destination),
		Format:          strings.ToLower(strings.TrimSpace(payload.Format)),
		Exclude:         payload.Exclude,
		IncludeMetadata: includeMetadata,
	}, nil
}
