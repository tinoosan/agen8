package agent

import "github.com/tinoosan/workbench-core/pkg/llm"

// FinalAnswerTool returns the control tool that ends the current turn.
func FinalAnswerTool() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "final_answer",
			Description: "[CONTROL] End the current turn with the final user-visible response text (markdown allowed) and optional deliverable artifact paths.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Final response text to show the user (markdown allowed)."},
					"artifacts": map[string]any{
						"type":        "array",
						"description": "Optional deliverable artifact paths (VFS paths).",
						"items":       map[string]any{"type": "string"},
					},
				},
				// Some providers (notably Azure) validate strict function schemas by requiring every property
				// to appear in `required`. `artifacts` remains semantically optional: callers can pass an empty array.
				"required":             []any{"text", "artifacts"},
				"additionalProperties": false,
			},
		},
	}
}
