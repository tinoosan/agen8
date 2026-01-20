package agent

import "github.com/tinoosan/workbench-core/internal/types"

// HostOpFunctions returns the core host primitives as function tool definitions.
//
// Tool names intentionally use underscores to match common function-calling conventions.
func HostOpFunctions() []types.Tool {
	// Helpers for strict-mode schemas:
	// - additionalProperties=false on objects
	// - all properties listed in required
	// - optional fields modeled via ["type","null"] and still listed in required
	intOrNull := []any{"integer", "null"}
	boolOrNull := []any{"boolean", "null"}

	return []types.Tool{
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_list",
				Description: "[DIRECT - no discovery needed] List directory contents at a VFS path. Common paths: /workdir (user's project), /workspace (your scratch), /tools (for tool_run discovery only).",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to list (e.g. /tools, /workspace)"},
					},
					"required":             []any{"path"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_read",
				Description: "[DIRECT - no discovery needed] Read file contents from a VFS path.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":     map[string]any{"type": "string", "description": "VFS path to read (e.g. /tools/<toolId>)"},
						"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to read (or null for default)"},
					},
					"required":             []any{"path", "maxBytes"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_write",
				Description: "[DIRECT - no discovery needed] Write/create a file at a VFS path. Just call it with path and text.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to write"},
						"text": map[string]any{"type": "string", "description": "File contents to write"},
					},
					"required":             []any{"path", "text"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_append",
				Description: "[DIRECT - no discovery needed] Append text to a file at a VFS path.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to append to"},
						"text": map[string]any{"type": "string", "description": "Text to append"},
					},
					"required":             []any{"path", "text"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_edit",
				Description: "[DIRECT - no discovery needed] Apply find-replace edits to a file. Each edit has old (exact match), new (replacement), occurrence (1-based).",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to edit"},
						"edits": map[string]any{
							"type":        "array",
							"description": "Edits to apply, in order.",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"old":        map[string]any{"type": "string"},
									"new":        map[string]any{"type": "string"},
									"occurrence": map[string]any{"type": "integer"},
								},
								"required":             []any{"old", "new", "occurrence"},
								"additionalProperties": false,
							},
						},
					},
					"required":             []any{"path", "edits"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_patch",
				Description: "[DIRECT - no discovery needed] Apply a unified diff patch to a file.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to patch"},
						"text": map[string]any{"type": "string", "description": "Unified diff text"},
					},
					"required":             []any{"path", "text"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "tool_run",
				Description: "[REQUIRES DISCOVERY] Run an external tool (bash, http, ripgrep, etc). BEFORE calling: (1) fs_read('/tools/<toolId>') to get the manifest and learn required input fields. Only then call with correct input. For simple file ops, use fs_write/fs_read directly instead.",
				// NOTE: tool_run.input is tool-defined arbitrary JSON. Some providers (e.g. Azure)
				// reject strict function schemas when they include arbitrary object properties.
				Strict: false,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"toolId":   map[string]any{"type": "string", "description": "Tool ID from fs_list('/tools'), e.g. 'builtin.http', 'builtin.bash'. Must be discovered first."},
						"actionId": map[string]any{"type": "string", "description": "Action ID from the tool manifest's 'actions' array (read via fs_read('/tools/<toolId>')). Example: 'fetch' for builtin.http."},
						"input": map[string]any{
							"type":                 "object",
							"description":          "Input object with fields matching the manifest's inputSchema. You MUST read the manifest first to know required fields (e.g., builtin.http fetch requires 'url').",
							"additionalProperties": true,
						},
						"timeoutMs": map[string]any{"type": intOrNull, "description": "Timeout in milliseconds (or null for default)"},
					},
					"required":             []any{"toolId", "actionId", "input", "timeoutMs"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "batch",
				Description: "[DIRECT - no discovery needed] Execute 2+ host ops in one call. Use dotted op names: fs.list, fs.read, fs.write, fs.append, fs.edit, fs.patch, tool.run. For tool.run in batch, you must have read the manifest first.",
				// NOTE: batch.operations items may include arbitrary nested objects (e.g. tool.run.input),
				// so strict tool schemas are not universally accepted by providers.
				Strict: false,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"parallel": map[string]any{"type": boolOrNull, "description": "Run operations in parallel when true (or null for default false)."},
						"operations": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"op": map[string]any{
										"type":        "string",
										"description": "Host op name (dotted, NOT underscore). Example: \"fs.write\" (NOT \"fs_write\").",
										"enum": []any{
											"fs.list",
											"fs.read",
											"fs.write",
											"fs.append",
											"fs.edit",
											"fs.patch",
											"tool.run",
										},
									},
									// Common fields (required depending on op):
									"path":     map[string]any{"type": "string", "description": "VFS path for fs.* ops (required for fs.list/fs.read/fs.write/fs.append/fs.edit/fs.patch)."},
									"text":     map[string]any{"type": "string", "description": "Text for fs.write/fs.append/fs.patch (required for those ops)."},
									"maxBytes": map[string]any{"type": "integer", "description": "Max bytes for fs.read (optional)."},
									"toolId":   map[string]any{"type": "string", "description": "Tool ID for tool.run (required for tool.run)."},
									"actionId": map[string]any{"type": "string", "description": "Action ID for tool.run (required for tool.run)."},
									"input": map[string]any{
										"type":                 "object",
										"description":          "Input object for fs.edit or tool.run (required for fs.edit and tool.run).",
										"additionalProperties": true,
									},
									"timeoutMs": map[string]any{"type": intOrNull, "description": "Timeout for tool.run (required for tool.run; or null)."},
								},
								"required":             []any{"op"},
								"additionalProperties": true,
							},
						},
					},
					"required":             []any{"parallel", "operations"},
					"additionalProperties": false,
				},
			},
		},
	}
}
