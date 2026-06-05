package decision

import (
	"encoding/json"
	"fmt"
)

const Name = "decision"
const Description = "[DECISIONS] Work-memory gateway for logging consequential choices."

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":                  map[string]any{"type": "string", "enum": []string{"log"}},
			"title":                   stringSchema("Decision title."),
			"rationale":               stringSchema("Explain why this decision was made."),
			"context":                 stringSchema("Relevant context for the decision."),
			"alternatives_rejected":   stringSchema("Rejected alternatives for action=log."),
			"invalidation_conditions": stringArraySchema("Conditions that would invalidate a logged decision."),
			"confidence":              numberSchema("Decision confidence from 0.0 to 1.0."),
			"task_ref":                stringSchema("Related task id."),
			"key_result_ref":          stringSchema("Related key result id."),
			"mission_ref":             stringSchema("Related mission id."),
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("decision schema encode: %v", err))
	}
	return body
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}
