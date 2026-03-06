package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSArchiveListTool lists archive entries without extracting.
type FSArchiveListTool struct{}

func (t *FSArchiveListTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_archive_list",
		"[DIRECT] List entries in an archive at a VFS path.",
		map[string]any{
			"path":  map[string]any{"type": "string", "description": "Absolute VFS archive path."},
			"limit": map[string]any{"type": "integer", "description": "Maximum entries to return. Default 200."},
		},
		[]any{"path"},
	)
}

func (t *FSArchiveListTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path  string `json:"path"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:    types.HostOpFSArchiveList,
		Path:  resolveVFSPath(payload.Path),
		Limit: payload.Limit,
	}, nil
}
