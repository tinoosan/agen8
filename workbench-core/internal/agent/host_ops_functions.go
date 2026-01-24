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
	stringOrNull := []any{"string", "null"}
	boolOrNull := []any{"boolean", "null"}

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
				Description: "[DIRECT - no discovery needed] List directory contents at a VFS path. Common paths: /project (user's project), /scratch (your scratch), /plan (planning workspace), /skills (skill documents), /tools (for tool_run discovery only).",
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
				Description: "[DIRECT - no discovery needed] Read file contents from a VFS path (skills live under /skills/<skill>/SKILL.md).",
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
				Name:        "update_plan",
				Description: "[DIRECT] Overwrite /plan/HEAD.md with the provided markdown plan checklist.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"plan": map[string]any{"type": "string", "description": "Checklist-style plan (markdown) to write to /plan/HEAD.md"},
					},
					"required":             []any{"plan"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "update_narrative",
				Description: "[DIRECT] Overwrite /plan/PLAN.md with narrative planning text.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string", "description": "Narrative plan text to write to /plan/PLAN.md"},
					},
					"required":             []any{"text"},
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
				Name:        "shell_exec",
				Description: "[CORE] Execute a shell command via bash. Supports pipes, redirects, and full shell syntax. Returns stdout, stderr, and exit code.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "Shell command to execute (e.g., \"ls -la | grep foo\")."},
						"cwd":     map[string]any{"type": stringOrNull, "description": "Working directory relative to the project root (e.g., \"internal/tools\"; default: \".\"). Do not use /project paths here."},
						"stdin":   map[string]any{"type": stringOrNull, "description": "Standard input to pipe to the command."},
					},
					"required":             []any{"command", "cwd", "stdin"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "http_fetch",
				Description: "[CORE] Make an HTTP request. Returns status, headers, and body.",
				// NOTE: headers is a free-form object (map[string]string) - strict mode
				// requires additionalProperties=false on all objects, which we can't do.
				Strict: false,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url":             map[string]any{"type": "string", "description": "URL to fetch."},
						"method":          map[string]any{"type": stringOrNull, "description": "HTTP method (default: GET)."},
						"headers":         map[string]any{"type": []any{"object", "null"}, "description": "Request headers."},
						"body":            map[string]any{"type": stringOrNull, "description": "Request body."},
						"maxBytes":        map[string]any{"type": intOrNull, "description": "Max response bytes."},
						"followRedirects": map[string]any{"type": boolOrNull, "description": "Follow redirects (default: true)."},
					},
					"required":             []any{"url"},
					"additionalProperties": false,
				},
			},
		},
		// Trace functions - provide insight into system events
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "trace_events_since",
				Description: "[CORE] Get system events since a cursor position. Returns events with pagination cursor for follow-up queries.",
				Strict:      false, // cursor can be string or number
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cursor":   map[string]any{"type": []any{"string", "integer"}, "description": "Cursor position (0 for start, or cursor from previous response)."},
						"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to return."},
						"limit":    map[string]any{"type": intOrNull, "description": "Max number of events."},
					},
					"required":             []any{"cursor"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "trace_events_latest",
				Description: "[CORE] Get the most recent system events. Good for quick insight into what just happened.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"maxBytes": map[string]any{"type": intOrNull, "description": "Max bytes to return."},
						"limit":    map[string]any{"type": intOrNull, "description": "Max number of events (default: 10)."},
					},
					"required":             []any{"maxBytes", "limit"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "trace_events_summary",
				Description: "[CORE] Get a summary of system events: event type counts, last timestamp, and recent event messages.",
				Strict:      false, // cursor can be string or number
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"cursor":       map[string]any{"type": []any{"string", "integer"}, "description": "Cursor position (0 for start)."},
						"maxBytes":     map[string]any{"type": intOrNull, "description": "Max bytes to scan."},
						"limit":        map[string]any{"type": intOrNull, "description": "Max events to scan."},
						"includeTypes": map[string]any{"type": []any{"array", "null"}, "items": map[string]any{"type": "string"}, "description": "Filter to specific event types."},
					},
					"required":             []any{"cursor"},
					"additionalProperties": false,
				},
			},
		},
		{
			Type: "function",
			Function: types.ToolFunction{
				Name:        "tool_run",
				Description: "[REQUIRES DISCOVERY] Run an external tool (e.g., custom tools provided via /tools). BEFORE calling: (1) fs_read('/tools/<toolId>') to get the manifest and learn required input fields. Only then call with correct input. For simple file ops, use fs_write/fs_read directly instead.",
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
