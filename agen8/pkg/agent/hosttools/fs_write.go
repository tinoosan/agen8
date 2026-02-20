package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSWriteTool writes a file in the VFS.
type FSWriteTool struct{}

func (t *FSWriteTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_write",
		"[DIRECT] Write/create a file at a VFS path. Typical target: /project/...",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "VFS path to write"},
			"text": map[string]any{"type": "string", "description": "File contents to write"},
		},
		[]any{"path", "text"},
	)
}

func (t *FSWriteTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	return fsPathTextExecute(types.HostOpFSWrite, args)
}
