package agent

import llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"

// FinalAnswerTool returns the control tool that ends the current turn.
func FinalAnswerTool() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "final_answer",
			Description: "[CONTROL] End the current turn with the final user-visible response text (markdown allowed), an explicit status, and optional deliverable artifact paths.",
			Strict:      true,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string", "description": "Final response text to show the user (markdown allowed)."},
					"status": map[string]any{
						"type":        "string",
						"description": "Execution status for this task/turn.",
						"enum":        []any{"succeeded", "failed"},
					},
					"error": map[string]any{"type": "string", "description": "Error summary when status='failed' (empty string otherwise)."},
					"artifacts": map[string]any{
						"type":        "array",
						"description": "Optional deliverable artifact paths (VFS paths).",
						"items":       map[string]any{"type": "string"},
					},
				},
				// Some providers (notably Azure) validate strict function schemas by requiring every property
				// to appear in `required`. `artifacts` remains semantically optional: callers can pass an empty array.
				"required":             []any{"text", "status", "error", "artifacts"},
				"additionalProperties": false,
			},
		},
	}
}
