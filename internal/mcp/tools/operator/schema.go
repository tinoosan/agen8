package operator

import (
	"encoding/json"
	"fmt"
)

const Name = "operator"
const Description = "[OPERATOR] Request human/operator work or escalate a decision gate, with graph-linked task/KR/mission context."

var allActions = []string{"request", "escalate"}

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":                map[string]any{"type": "string", "enum": allActions},
			"title":                 stringSchema("Short operator-facing title."),
			"description":           stringSchema("Full context for the operator."),
			"recommendation":        stringSchema("Required for action=escalate. Recommended decision or next move."),
			"category":              stringSchema("Operator category: financial, legal, content, code, general, physical, communication, administrative."),
			"urgency":               stringSchema("Operator urgency: low, medium, high, critical."),
			"confidence":            numberSchema("Escalation confidence from 0.0 to 1.0."),
			"task_ref":              stringSchema("Related task id."),
			"key_result_ref":        stringSchema("Related key result id."),
			"mission_ref":           stringSchema("Related mission id."),
			"run_id":                stringSchema("Related harness run/session id for action=request."),
			"blocking":              booleanSchema("For action=request, whether the action blocks task progress."),
			"requires_verification": booleanSchema("For action=request, whether the agent must verify the operator outcome."),
			"deadline_hours":        integerSchema("Optional relative deadline in hours."),
			"metadata":              stringMap("Optional metadata for operator state. Values must be strings."),
		},
		"required":             []string{"action", "title", "description", "category", "urgency"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("operator schema encode: %v", err))
	}
	return body
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func booleanSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func stringMap(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
		"description":          description,
	}
}
