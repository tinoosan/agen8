package decision

import (
	"encoding/json"
	"fmt"
)

const Name = "decision"
const Description = "[DECISIONS] Work-memory gateway for logging consequential choices and resolving structured human questions."

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":                  map[string]any{"type": "string", "enum": []string{"log", "ask_user"}},
			"title":                   stringSchema("Decision title or human-input prompt title. Required for log and ask_user."),
			"rationale":               stringSchema("Required for action=log. Explain why this decision was made."),
			"context":                 stringSchema("Context for action=ask_user."),
			"alternatives_rejected":   stringSchema("Rejected alternatives for action=log."),
			"invalidation_conditions": stringArraySchema("Conditions that would invalidate a logged decision."),
			"confidence":              numberSchema("Decision confidence from 0.0 to 1.0."),
			"task_ref":                stringSchema("Related task id."),
			"key_result_ref":          stringSchema("Related key result id."),
			"mission_ref":             stringSchema("Related mission id."),
			"plan_ref":                stringSchema("Related plan id."),
			"questions":               questionsArraySchema("Required for action=ask_user."),
			"answers":                 answersArraySchema("Resolved answers when completing a human-input request directly."),
			"cancelled":               booleanSchema("Whether the human-input request was cancelled."),
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

func booleanSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}

func questionsArraySchema(description string) map[string]any {
	return map[string]any{
		"type":     "array",
		"minItems": 1,
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             map[string]any{"type": "string"},
				"text":           map[string]any{"type": "string"},
				"type":           map[string]any{"type": "string"},
				"options":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"allowFreeForm":  map[string]any{"type": "boolean"},
				"recommendation": map[string]any{"type": "string"},
				"blocking":       map[string]any{"type": "boolean"},
			},
			"required":             []string{"id", "text", "type"},
			"additionalProperties": false,
		},
		"description": description,
	}
}

func answersArraySchema(description string) map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questionId":      map[string]any{"type": "string"},
				"selectedOption":  map[string]any{"type": "string"},
				"selectedOptions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"freeFormText":    map[string]any{"type": "string"},
			},
			"required":             []string{"questionId"},
			"additionalProperties": false,
		},
		"description": description,
	}
}
