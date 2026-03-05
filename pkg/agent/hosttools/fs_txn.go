package hosttools

import (
	"context"
	"encoding/json"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSTxnTool applies multiple mutating fs_* steps atomically.
type FSTxnTool struct{}

func (t *FSTxnTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_txn",
		"[DIRECT] Execute multiple mutating fs_* steps atomically. Prefer dryRun=true first, then apply=true to commit.",
		map[string]any{
			"steps": map[string]any{
				"type":        "array",
				"description": "Ordered mutating steps. Supported ops: fs_write, fs_append, fs_edit, fs_patch.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"op":               map[string]any{"type": "string", "enum": []string{"fs_write", "fs_append", "fs_edit", "fs_patch"}},
						"path":             map[string]any{"type": "string"},
						"text":             map[string]any{"type": "string"},
						"input":            map[string]any{"type": "object"},
						"mode":             map[string]any{"type": "string", "enum": []string{"w", "a"}},
						"verify":           map[string]any{"type": "boolean"},
						"checksum":         map[string]any{"type": "string"},
						"checksumExpected": map[string]any{"type": "string"},
						"atomic":           map[string]any{"type": "boolean"},
						"sync":             map[string]any{"type": "boolean"},
						"verbose":          map[string]any{"type": "boolean"},
					},
					"required":             []any{"op", "path"},
					"additionalProperties": false,
				},
			},
			"options": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dryRun":          map[string]any{"type": "boolean", "description": "Validate/simulate steps without writing. Default true."},
					"apply":           map[string]any{"type": "boolean", "description": "Apply changes. Default false."},
					"rollbackOnError": map[string]any{"type": "boolean", "description": "Rollback touched files if apply fails. Default true."},
				},
				"additionalProperties": false,
			},
		},
		[]any{"steps"},
	)
}

func (t *FSTxnTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Steps []struct {
			Op               string          `json:"op"`
			Path             string          `json:"path"`
			Text             string          `json:"text"`
			Input            json.RawMessage `json:"input"`
			Mode             string          `json:"mode"`
			Verify           bool            `json:"verify"`
			Checksum         string          `json:"checksum"`
			ChecksumExpected string          `json:"checksumExpected"`
			Atomic           bool            `json:"atomic"`
			Sync             bool            `json:"sync"`
			Verbose          bool            `json:"verbose"`
		} `json:"steps"`
		Options *struct {
			DryRun          bool `json:"dryRun"`
			Apply           bool `json:"apply"`
			RollbackOnError bool `json:"rollbackOnError"`
		} `json:"options"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}

	steps := make([]types.FSTxnStep, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		steps = append(steps, types.FSTxnStep{
			Op:               strings.ToLower(strings.TrimSpace(step.Op)),
			Path:             resolveVFSPath(step.Path),
			Text:             step.Text,
			Input:            step.Input,
			Mode:             step.Mode,
			Verify:           step.Verify,
			Checksum:         step.Checksum,
			ChecksumExpected: step.ChecksumExpected,
			Atomic:           step.Atomic,
			Sync:             step.Sync,
			Verbose:          step.Verbose,
		})
	}

	var options *types.FSTxnOptions
	if payload.Options != nil {
		options = &types.FSTxnOptions{
			DryRun:          payload.Options.DryRun,
			Apply:           payload.Options.Apply,
			RollbackOnError: payload.Options.RollbackOnError,
		}
	}

	return types.HostOpRequest{
		Op:         types.HostOpFSTxn,
		TxnSteps:   steps,
		TxnOptions: options,
	}, nil
}
