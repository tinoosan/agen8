package agent

import "github.com/tinoosan/workbench-core/internal/types"

// hostOpResponseSchema returns the Structured Outputs schema for the agent loop v0:
// the model must return exactly one JSON object that is either:
// - a HostOpRequest (fs.* / tool.run / final), or
// - a HostOpBatchRequest (batch of HostOpRequest operations, excluding final).
func hostOpResponseSchema() *types.LLMResponseSchema {
	// IMPORTANT:
	// Structured Outputs supports only a subset of JSON Schema in strict mode, and
	// some OpenAI-compatible providers reject advanced constructs like oneOf.
	//
	// Keep the schema intentionally minimal: enforce that the model returns a single
	// JSON object with a required "op" and known fields. We do not attempt to encode
	// per-op required fields here; host-side validation still enforces that.
	schema := map[string]any{
		"type":     "object",
		"required": []any{"op"},
		// Avoid degenerate outputs like {"op":"fs.list"} with no required companion fields.
		// This stays within the common subset supported by many providers.
		"minProperties": 2,
		"properties": map[string]any{
			// IMPORTANT: keep this extremely small for provider compatibility.
			// Host-side validation enforces allowed values and required companion fields.
			"op": map[string]any{
				"type": "string",
			},
			"path": map[string]any{
				"type": "string",
			},
			"text": map[string]any{
				"type": "string",
			},
			"toolId": map[string]any{
				"type": "string",
			},
			"actionId": map[string]any{
				"type": "string",
			},
			"input": map[string]any{
				"type": "object",
			},
			"timeoutMs": map[string]any{
				"type": "integer",
			},
			"maxBytes": map[string]any{
				"type": "integer",
			},
			"parallel": map[string]any{
				"type": "boolean",
			},
			"operations": map[string]any{
				"type": "array",
			},
			"ops": map[string]any{
				"type": "array",
			},
		},
	}

	return &types.LLMResponseSchema{
		Name:        "workbench_host_op",
		Description: "Workbench agent host operation (one JSON object per turn).",
		Schema:      schema,
		Strict:      false, // omit strict to maximize provider compatibility
	}
}
