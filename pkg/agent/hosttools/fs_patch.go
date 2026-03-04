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
			"path":    map[string]any{"type": "string", "description": "VFS path to patch"},
			"text":    map[string]any{"type": "string", "description": "Unified diff text"},
			"dryRun":  map[string]any{"type": "boolean", "description": "Validate patch and return diagnostics without writing changes."},
			"verbose": map[string]any{"type": "boolean", "description": "Include richer diagnostics (context snippets and hints) when possible."},
		},
		[]any{"path", "text"},
	)
}

func (t *FSPatchTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path    string `json:"path"`
		Text    string `json:"text"`
		DryRun  bool   `json:"dryRun"`
		Verbose bool   `json:"verbose"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:      types.HostOpFSPatch,
		Path:    resolveVFSPath(payload.Path),
		Text:    payload.Text,
		DryRun:  payload.DryRun,
		Verbose: payload.Verbose,
	}, nil
}
