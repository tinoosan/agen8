package harnessapproval

import (
	"encoding/json"
	"fmt"
)

const Name = "harness_approval"
const Description = "[HARNESS] Internal approval gate for native harness permission prompts."

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool_name": map[string]any{"type": "string", "description": "Native harness tool name requesting approval."},
			"tool_input": map[string]any{
				"type":                 "object",
				"description":          "Native harness tool input requesting approval.",
				"additionalProperties": true,
			},
			"input": map[string]any{
				"type":                 "object",
				"description":          "Alternate native harness input field used by some Claude Code versions.",
				"additionalProperties": true,
			},
			"tool_use_id": map[string]any{"type": "string", "description": "Native harness tool use id."},
			"id":          map[string]any{"type": "string", "description": "Native harness approval or tool id."},
			"cwd":         map[string]any{"type": "string", "description": "Working directory for the requested action."},
			"reason":      map[string]any{"type": "string", "description": "Native harness explanation for the requested action."},
		},
		"required":             []string{},
		"additionalProperties": true,
	})
	if err != nil {
		panic(fmt.Sprintf("harness approval schema encode: %v", err))
	}
	return body
}
