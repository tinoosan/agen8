package agent

import "github.com/tinoosan/workbench-core/pkg/llm"

// HostOpFunctions returns the core host primitives as function tool definitions.
func HostOpFunctions() []llm.Tool {
	intOrNull := []any{"integer", "null"}
	stringOrNull := []any{"string", "null"}
	boolOrNull := []any{"boolean", "null"}

	return []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
				Name:        "update_plan",
				Description: "[DIRECT] Overwrite /plan/HEAD.md with the checklist. Use this at the start of multi-step work AND after each meaningful step to keep progress current.",
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
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
			Function: llm.ToolFunction{
				Name:        "http_fetch",
				Description: "[CORE] Make an HTTP request. Returns status, headers, and body.",
				Strict:      false,
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
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "tool_run",
				Description: "[DISCOVERY] Run a discovered tool by id + action. Use fs_list/fs_read on /tools to discover manifests.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"toolId":   map[string]any{"type": "string", "description": "Tool id to run (e.g. \"builtin.shell\" or custom)."},
						"actionId": map[string]any{"type": "string", "description": "Action id to run (from manifest)."},
						"input": map[string]any{
							"type":                 "object",
							"description":          "Action input as JSON object.",
							"properties":           map[string]any{},
							"additionalProperties": true,
						},
						"timeoutMs": map[string]any{"type": intOrNull, "description": "Timeout override in ms (or null to use default)."},
					},
					"required":             []string{"toolId", "actionId", "input", "timeoutMs"},
					"additionalProperties": false,
				},
			},
		},
	}
}
