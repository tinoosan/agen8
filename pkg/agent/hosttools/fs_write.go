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
		"[DIRECT] Write/create a file at a VFS path. Typical target: /project/... Optional write safety controls: verify/checksum/atomic/sync.",
		map[string]any{
			"path":     map[string]any{"type": "string", "description": "VFS path to write"},
			"text":     map[string]any{"type": "string", "description": "File contents to write"},
			"verify":   map[string]any{"type": "boolean", "description": "Read back and compare after writing."},
			"checksum": map[string]any{"type": "string", "enum": []string{"md5", "sha1", "sha256"}, "description": "Optional checksum algorithm for the written content."},
			"atomic":   map[string]any{"type": "boolean", "description": "Request atomic write semantics (best-effort by mount)."},
			"sync":     map[string]any{"type": "boolean", "description": "Request fsync durability semantics (best-effort by mount)."},
		},
		[]any{"path", "text"},
	)
}

func (t *FSWriteTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path     string `json:"path"`
		Text     string `json:"text"`
		Verify   bool   `json:"verify"`
		Checksum string `json:"checksum"`
		Atomic   bool   `json:"atomic"`
		Sync     bool   `json:"sync"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:       types.HostOpFSWrite,
		Path:     resolveVFSPath(payload.Path),
		Text:     payload.Text,
		Verify:   payload.Verify,
		Checksum: payload.Checksum,
		Atomic:   payload.Atomic,
		Sync:     payload.Sync,
	}, nil
}
