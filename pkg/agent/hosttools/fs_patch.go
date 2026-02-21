package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSPatchTool applies a unified diff patch to a file in the VFS.
type FSPatchTool struct{}

func (t *FSPatchTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_patch",
		"[DIRECT - no discovery needed] Apply a unified diff patch to a file.",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "VFS path to patch"},
			"text": map[string]any{"type": "string", "description": "Unified diff text"},
		},
		[]any{"path", "text"},
	)
}

func (t *FSPatchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	return fsPathTextExecute(types.HostOpFSPatch, args)
}
