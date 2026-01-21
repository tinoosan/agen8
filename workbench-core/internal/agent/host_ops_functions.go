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

	return []types.Tool{
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "final_answer",
				Description: "[CONTROL] End the current turn with the final user-visible response text (markdown allowed). Use this when you are fully done and no more environment actions are needed.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string", "description": "Final response text to show the user (markdown allowed)."},
					},
					"required":             []any{"text"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "fs_list",
				Description: "[DIRECT - no discovery needed] List directory contents at a VFS path. Common paths: /project (user's project), /scratch (your scratch), /tools (for tool_run discovery only).",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "VFS path to list (e.g. /tools, /scratch)"},
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
				Description: "[REQUIRES DISCOVERY] Run an external tool (e.g., builtin.trace or custom tools). BEFORE calling: (1) fs_read('/tools/<toolId>') to get the manifest and learn required input fields. Only then call with correct input. For simple file ops, use fs_write/fs_read directly instead.",
				// NOTE: tool_run.input is tool-defined arbitrary JSON. Some providers (e.g. Azure)
				// reject strict function schemas when they include arbitrary object properties.
				Strict: false,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"toolId":   map[string]any{"type": "string", "description": "Tool ID from fs_list('/tools'), e.g. 'builtin.http', 'builtin.shell'. Must be discovered first."},
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
	}
}
