package agent

import llmtypes "github.com/tinoosan/agen8/pkg/llm/types"

// hostOpResponseSchema returns the Structured Outputs schema for the agent loop v0.
func hostOpResponseSchema() *llmtypes.LLMResponseSchema {
	schema := map[string]any{
		"type":          "object",
		"required":      []any{"op"},
		"minProperties": 2,
		"properties": map[string]any{
			"op":         map[string]any{"type": "string"},
			"path":       map[string]any{"type": "string"},
			"text":       map[string]any{"type": "string"},
			"toolId":     map[string]any{"type": "string"},
			"actionId":   map[string]any{"type": "string"},
			"input":      map[string]any{"type": "object"},
			"timeoutMs":  map[string]any{"type": "integer"},
			"maxBytes":   map[string]any{"type": "integer"},
			"parallel":   map[string]any{"type": "boolean"},
			"operations": map[string]any{"type": "array"},
			"ops":        map[string]any{"type": "array"},
		},
	}

	return &llmtypes.LLMResponseSchema{
		Name:        "agen8_host_op",
		Description: "Agen8 agent host operation (one JSON object per turn).",
		Schema:      schema,
		Strict:      false,
	}
}
