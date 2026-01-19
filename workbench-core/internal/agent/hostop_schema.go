package agent

import "github.com/tinoosan/workbench-core/internal/types"

// hostOpResponseSchema returns the Structured Outputs schema for the agent loop v0:
// the model must return exactly one JSON object that is either:
// - a HostOpRequest (fs.* / tool.run / final), or
// - a HostOpBatchRequest (batch of HostOpRequest operations, excluding final).
func hostOpResponseSchema() *types.LLMResponseSchema {
	// Keep this schema within the “structured outputs” subset (provider-specific).
	// Prefer simple object/array/string/number/bool/null constructs, and keep
	// additionalProperties=false to reduce shape drift.
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"op": map[string]any{
				"type": "string",
				"enum": []any{
					types.HostOpFSList,
					types.HostOpFSRead,
					types.HostOpFSWrite,
					types.HostOpFSAppend,
					types.HostOpFSEdit,
					types.HostOpFSPatch,
					types.HostOpToolRun,
					types.HostOpFinal,
					types.HostOpBatch,
				},
			},
			// HostOpRequest fields
			"path":      map[string]any{"type": "string"},
			"toolId":    map[string]any{"type": "string"},
			"actionId":  map[string]any{"type": "string"},
			"input":     map[string]any{"type": "object"},
			"timeoutMs": map[string]any{"type": "integer", "minimum": 0},
			"maxBytes":  map[string]any{"type": "integer", "minimum": 0},
			"text":      map[string]any{"type": "string"},
			// HostOpBatchRequest fields
			"operations": map[string]any{
				"type":  "array",
				"items": hostOpRequestNonFinalSchema(),
				"minItems": 1,
			},
			"parallel": map[string]any{"type": "boolean"},
		},
		"oneOf": []any{
			hostOpRequestSchema(),
			hostOpBatchSchema(),
		},
	}

	return &types.LLMResponseSchema{
		Name:        "workbench_host_op",
		Description: "Workbench agent host operation (one JSON object per turn).",
		Schema:      schema,
		Strict:      true,
	}
}

func hostOpRequestSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"op"},
		"oneOf": []any{
			map[string]any{
				"properties": map[string]any{
					"op":       map[string]any{"enum": []any{types.HostOpFinal}},
					"text":     map[string]any{"type": "string"},
					"path":     map[string]any{"type": "string"},
					"toolId":   map[string]any{"type": "string"},
					"actionId": map[string]any{"type": "string"},
					"input":    map[string]any{"type": "object"},
				},
				"required": []any{"op", "text"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":      map[string]any{"enum": []any{types.HostOpFSList, types.HostOpFSRead}},
					"path":    map[string]any{"type": "string"},
					"maxBytes": map[string]any{"type": "integer", "minimum": 0},
				},
				"required": []any{"op", "path"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSWrite, types.HostOpFSAppend}},
					"path": map[string]any{"type": "string"},
					"text": map[string]any{"type": "string"},
				},
				"required": []any{"op", "path", "text"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSEdit}},
					"path": map[string]any{"type": "string"},
					"input": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"edits": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":                 "object",
									"additionalProperties": false,
									"properties": map[string]any{
										"old":        map[string]any{"type": "string"},
										"new":        map[string]any{"type": "string"},
										"occurrence": map[string]any{"type": "integer", "minimum": 1},
									},
									"required": []any{"old", "new", "occurrence"},
								},
								"minItems": 1,
							},
						},
						"required": []any{"edits"},
					},
				},
				"required": []any{"op", "path", "input"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSPatch}},
					"path": map[string]any{"type": "string"},
					"text": map[string]any{"type": "string"},
				},
				"required": []any{"op", "path", "text"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":       map[string]any{"enum": []any{types.HostOpToolRun}},
					"toolId":   map[string]any{"type": "string"},
					"actionId": map[string]any{"type": "string"},
					"input":    map[string]any{"type": "object"},
					"timeoutMs": map[string]any{
						"type":    "integer",
						"minimum": 0,
					},
				},
				"required": []any{"op", "toolId", "actionId", "input"},
			},
		},
	}
}

func hostOpRequestNonFinalSchema() map[string]any {
	// Same as HostOpRequest, but excludes {"op":"final",...} and {"op":"batch",...}.
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"op"},
		"oneOf": []any{
			map[string]any{
				"properties": map[string]any{
					"op":      map[string]any{"enum": []any{types.HostOpFSList, types.HostOpFSRead}},
					"path":    map[string]any{"type": "string"},
					"maxBytes": map[string]any{"type": "integer", "minimum": 0},
				},
				"required": []any{"op", "path"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSWrite, types.HostOpFSAppend}},
					"path": map[string]any{"type": "string"},
					"text": map[string]any{"type": "string"},
				},
				"required": []any{"op", "path", "text"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSEdit}},
					"path": map[string]any{"type": "string"},
					"input": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"edits": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type":                 "object",
									"additionalProperties": false,
									"properties": map[string]any{
										"old":        map[string]any{"type": "string"},
										"new":        map[string]any{"type": "string"},
										"occurrence": map[string]any{"type": "integer", "minimum": 1},
									},
									"required": []any{"old", "new", "occurrence"},
								},
								"minItems": 1,
							},
						},
						"required": []any{"edits"},
					},
				},
				"required": []any{"op", "path", "input"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":   map[string]any{"enum": []any{types.HostOpFSPatch}},
					"path": map[string]any{"type": "string"},
					"text": map[string]any{"type": "string"},
				},
				"required": []any{"op", "path", "text"},
			},
			map[string]any{
				"properties": map[string]any{
					"op":       map[string]any{"enum": []any{types.HostOpToolRun}},
					"toolId":   map[string]any{"type": "string"},
					"actionId": map[string]any{"type": "string"},
					"input":    map[string]any{"type": "object"},
					"timeoutMs": map[string]any{
						"type":    "integer",
						"minimum": 0,
					},
				},
				"required": []any{"op", "toolId", "actionId", "input"},
			},
		},
	}
}

func hostOpBatchSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"op":         map[string]any{"enum": []any{types.HostOpBatch}},
			"operations": map[string]any{"type": "array", "items": hostOpRequestNonFinalSchema(), "minItems": 1},
			"parallel":   map[string]any{"type": "boolean"},
		},
		"required": []any{"op", "operations"},
	}
}

