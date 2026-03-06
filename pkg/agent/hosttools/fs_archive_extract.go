package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSArchiveExtractTool extracts an archive to a destination directory.
type FSArchiveExtractTool struct{}

func (t *FSArchiveExtractTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_archive_extract",
		"[DIRECT] Extract an archive at a VFS path into a destination directory.",
		map[string]any{
			"path":        map[string]any{"type": "string", "description": "Absolute VFS source archive path."},
			"destination": map[string]any{"type": "string", "description": "Absolute VFS destination directory path."},
			"overwrite":   map[string]any{"type": "boolean", "description": "Overwrite existing files. Default false."},
			"pattern":     map[string]any{"type": "string", "description": "Optional glob pattern over archive entry names."},
		},
		[]any{"path", "destination"},
	)
}

func (t *FSArchiveExtractTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path        string `json:"path"`
		Destination string `json:"destination"`
		Overwrite   bool   `json:"overwrite"`
		Pattern     string `json:"pattern"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:          types.HostOpFSArchiveExtract,
		Path:        resolveVFSPath(payload.Path),
		Destination: resolveVFSPath(payload.Destination),
		Overwrite:   payload.Overwrite,
		Pattern:     payload.Pattern,
	}, nil
}
