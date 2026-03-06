package hosttools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSBatchEditTool applies the same structured edits across multiple files.
type FSBatchEditTool struct{}

func (t *FSBatchEditTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_batch_edit",
		"[DIRECT] Apply the same exact-match edits across many files selected by path + glob. Prefer dryRun=true first, then apply=true to commit atomically.",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "Absolute VFS root to search under."},
			"glob": map[string]any{"type": "string", "description": "Required include glob relative to path."},
			"exclude": map[string]any{
				"description": "Optional exclude glob or glob list.",
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			"edits": map[string]any{
				"type":        "array",
				"description": "Exact-match edits to apply to each matching file.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old": map[string]any{"type": "string"},
						"new": map[string]any{"type": "string"},
						"occurrence": map[string]any{
							"oneOf": []any{
								map[string]any{"type": "string", "enum": []string{"all"}},
								map[string]any{"type": "integer", "minimum": 1},
							},
						},
					},
					"required":             []any{"old", "new"},
					"additionalProperties": false,
				},
			},
			"options": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dryRun":          map[string]any{"type": "boolean", "description": "Preview matches without writing. Default true."},
					"apply":           map[string]any{"type": "boolean", "description": "Apply edits. Default false."},
					"rollbackOnError": map[string]any{"type": "boolean", "description": "Rollback touched files if apply fails. Default true."},
					"maxFiles":        map[string]any{"type": "integer", "description": "Safety cap on matched files. Default 1000."},
				},
				"additionalProperties": false,
			},
		},
		[]any{"path", "glob", "edits"},
	)
}

func (t *FSBatchEditTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path    string          `json:"path"`
		Glob    string          `json:"glob"`
		Exclude json.RawMessage `json:"exclude"`
		Edits   []struct {
			Old        string          `json:"old"`
			New        string          `json:"new"`
			Occurrence json.RawMessage `json:"occurrence"`
		} `json:"edits"`
		Options *struct {
			DryRun          bool `json:"dryRun"`
			Apply           bool `json:"apply"`
			RollbackOnError bool `json:"rollbackOnError"`
			MaxFiles        int  `json:"maxFiles"`
		} `json:"options"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}
	exclude, err := normalizeStringListArg(payload.Exclude)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	edits := make([]types.BatchEdit, 0, len(payload.Edits))
	for _, edit := range payload.Edits {
		occurrence := "all"
		if len(edit.Occurrence) != 0 {
			var s string
			if err := json.Unmarshal(edit.Occurrence, &s); err == nil {
				if strings.TrimSpace(s) != "" {
					occurrence = strings.TrimSpace(s)
				}
			} else {
				var n int
				if err := json.Unmarshal(edit.Occurrence, &n); err != nil {
					return types.HostOpRequest{}, err
				}
				occurrence = strconv.Itoa(n)
			}
		}
		edits = append(edits, types.BatchEdit{
			Old:        edit.Old,
			New:        edit.New,
			Occurrence: occurrence,
		})
	}

	var options *types.BatchOptions
	if payload.Options != nil {
		options = &types.BatchOptions{
			DryRun:          payload.Options.DryRun,
			Apply:           payload.Options.Apply,
			RollbackOnError: payload.Options.RollbackOnError,
			MaxFiles:        payload.Options.MaxFiles,
		}
	}

	return types.HostOpRequest{
		Op:               types.HostOpFSBatchEdit,
		Path:             resolveVFSPath(payload.Path),
		Glob:             strings.TrimSpace(payload.Glob),
		Exclude:          exclude,
		BatchEditEdits:   edits,
		BatchEditOptions: options,
	}, nil
}
