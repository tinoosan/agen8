package hosttools

import (
	"context"
	"encoding/json"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// PipeTool runs a simple declarative pipeline of tool calls and transforms.
type PipeTool struct{}

func (t *PipeTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "pipe",
			Description: "[DIRECT] Run a simple linear pipeline of allowed tool calls and built-in transforms without writing Python. Prefer this for straightforward read/transform/write flows; use code_exec for branching, loops, or custom logic.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"steps": map[string]any{
						"type":        "array",
						"description": "Ordered tool or transform steps.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type":        map[string]any{"type": "string", "enum": []string{"tool", "transform"}},
								"tool":        map[string]any{"type": "string"},
								"args":        map[string]any{"type": "object", "additionalProperties": true},
								"inputArg":    map[string]any{"type": "string"},
								"output":      map[string]any{"type": "string"},
								"transform":   map[string]any{"type": "string", "enum": []string{"uppercase", "lowercase", "trim", "json_parse", "json_stringify", "get", "join", "split", "regex_replace"}},
								"field":       map[string]any{"type": "string"},
								"separator":   map[string]any{"type": "string"},
								"pattern":     map[string]any{"type": "string"},
								"replacement": map[string]any{"type": "string"},
							},
							"required":             []any{"type"},
							"additionalProperties": false,
						},
					},
					"options": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"debug":         map[string]any{"type": "boolean"},
							"maxSteps":      map[string]any{"type": "integer"},
							"maxValueBytes": map[string]any{"type": "integer"},
						},
						"additionalProperties": false,
					},
				},
				"required":             []any{"steps"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *PipeTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Steps []struct {
			Type        string         `json:"type"`
			Tool        string         `json:"tool"`
			Args        map[string]any `json:"args"`
			InputArg    string         `json:"inputArg"`
			Output      string         `json:"output"`
			Transform   string         `json:"transform"`
			Field       string         `json:"field"`
			Separator   string         `json:"separator"`
			Pattern     string         `json:"pattern"`
			Replacement string         `json:"replacement"`
		} `json:"steps"`
		Options *struct {
			Debug         bool `json:"debug"`
			MaxSteps      int  `json:"maxSteps"`
			MaxValueBytes int  `json:"maxValueBytes"`
		} `json:"options"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}

	steps := make([]types.PipeStep, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		steps = append(steps, types.PipeStep{
			Type:        strings.ToLower(strings.TrimSpace(step.Type)),
			Tool:        strings.ToLower(strings.TrimSpace(step.Tool)),
			Args:        step.Args,
			InputArg:    strings.TrimSpace(step.InputArg),
			Output:      strings.TrimSpace(step.Output),
			Transform:   strings.ToLower(strings.TrimSpace(step.Transform)),
			Field:       strings.TrimSpace(step.Field),
			Separator:   step.Separator,
			Pattern:     step.Pattern,
			Replacement: step.Replacement,
		})
	}

	var options *types.PipeOptions
	if payload.Options != nil {
		options = &types.PipeOptions{
			Debug:         payload.Options.Debug,
			MaxSteps:      payload.Options.MaxSteps,
			MaxValueBytes: payload.Options.MaxValueBytes,
		}
	}

	return types.HostOpRequest{
		Op:          types.HostOpPipe,
		PipeSteps:   steps,
		PipeOptions: options,
	}, nil
}
