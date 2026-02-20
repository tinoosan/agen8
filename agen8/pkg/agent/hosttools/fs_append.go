package hosttools

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSAppendTool appends text to a file in the VFS.
type FSAppendTool struct{}

func (t *FSAppendTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_append",
		"[DIRECT - no discovery needed] Append text to a file at a VFS path.",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "VFS path to append to"},
			"text": map[string]any{"type": "string", "description": "Text to append"},
		},
		[]any{"path", "text"},
	)
}

func (t *FSAppendTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	return fsPathTextExecute(types.HostOpFSAppend, args)
}
