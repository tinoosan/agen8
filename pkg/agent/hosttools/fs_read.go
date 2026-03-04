package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/pkg/checksumutil"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSReadTool reads a file from the VFS.
type FSReadTool struct{}

func (t *FSReadTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_read",
		"[DIRECT - no discovery needed] Read file contents from a VFS path (skills live under /skills/<skill_name>/SKILL.md).",
		map[string]any{
			"path":     map[string]any{"type": "string", "description": "VFS path to read (e.g. /project/README.md)"},
			"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)"},
			"checksum": map[string]any{
				"description": "Optional checksum algorithm(s) to compute for read content.",
				"anyOf": []any{
					map[string]any{"type": "string", "enum": checksumutil.SupportedAlgorithms()},
					map[string]any{
						"type":        "array",
						"minItems":    1,
						"uniqueItems": true,
						"items":       map[string]any{"type": "string", "enum": checksumutil.SupportedAlgorithms()},
					},
				},
			},
		},
		[]any{"path", "maxBytes"},
	)
}

func (t *FSReadTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path     string `json:"path"`
		MaxBytes *int   `json:"maxBytes"`
		Checksum any    `json:"checksum"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	checksums, err := normalizeReadChecksumArgs(payload.Checksum)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	maxBytes := 1024 * 1024
	if payload.MaxBytes != nil {
		maxBytes = *payload.MaxBytes
	}
	return types.HostOpRequest{
		Op:        types.HostOpFSRead,
		Path:      resolveVFSPath(payload.Path),
		MaxBytes:  maxBytes,
		Checksums: checksums,
	}, nil
}

func normalizeReadChecksumArgs(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(algo string) error {
		algo = checksumutil.NormalizeAlgorithm(algo)
		if algo == "" {
			return fmt.Errorf("checksum entries must be non-empty")
		}
		if !checksumutil.IsSupportedAlgorithm(algo) {
			return fmt.Errorf("checksum must be one of %s", checksumutil.SupportedAlgorithmsDisplay())
		}
		if _, ok := seen[algo]; ok {
			return nil
		}
		seen[algo] = struct{}{}
		out = append(out, algo)
		return nil
	}

	switch v := raw.(type) {
	case string:
		if err := add(v); err != nil {
			return nil, err
		}
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("checksum array entries must be strings")
			}
			if err := add(s); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("checksum must be a string or array of strings")
	}

	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out, nil
}
