package agent

import "github.com/tinoosan/workbench-core/pkg/llm"

// FinalAnswerTool returns the control tool that ends the current turn.
func FinalAnswerTool() llm.Tool {
	return llm.Tool{
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
	}
}
