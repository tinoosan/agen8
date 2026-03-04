package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSStatTool returns filesystem metadata (type/size) for a VFS path.
type FSStatTool struct{}

func (t *FSStatTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_stat",
		"[DIRECT] Return metadata for a VFS path (file vs directory, and file size when available) without reading file content.",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "VFS path to inspect (e.g. /project/README.md)"},
		},
		[]any{"path"},
	)
}

func (t *FSStatTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSStat,
		Path: resolveVFSPath(payload.Path),
	}, nil
}
