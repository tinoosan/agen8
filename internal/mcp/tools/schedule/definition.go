package schedule

import (
	"encoding/json"
	"fmt"
)

const Name = "schedule"
const Description = "[SCHEDULE] Schedule future agent work; first target is task.create."

var allActions = []string{"create", "get", "list", "update", "cancel"}

func (h Handler) Schema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":              map[string]any{"type": "string", "enum": allActions},
			"schedule_id":         stringSchema("Schedule entry id for get, update, or cancel."),
			"title":               stringSchema("Schedule title."),
			"description":         stringSchema("Schedule description."),
			"mode":                map[string]any{"type": "string", "enum": []string{"once", "interval", "cron"}},
			"run_at":              stringSchema("RFC3339 timestamp for mode=once."),
			"interval_seconds":    integerSchema("Interval in seconds for mode=interval."),
			"cron":                stringSchema("Five-field cron expression or supported alias for mode=cron."),
			"timezone":            stringSchema("IANA timezone for mode=cron."),
			"target_kind":         map[string]any{"type": "string", "enum": []string{"task.create"}},
			"target_member_id":    stringSchema("Member id assigned when target_kind=task.create."),
			"task_title":          stringSchema("Task title for target_kind=task.create."),
			"task_description":    stringSchema("Task description for target_kind=task.create."),
			"acceptance_criteria": stringArraySchema("Task acceptance criteria."),
			"mission_ref":         stringSchema("Mission id linked to the scheduled task."),
			"key_result_ref":      stringSchema("Key result id linked to the scheduled task."),
			"status":              map[string]any{"type": "string", "enum": []string{"active", "paused", "triggered", "expired", "cancelled"}},
			"limit":               integerSchema("List limit. Maximum 50."),
			"expires_at":          stringSchema("Optional RFC3339 expiry timestamp."),
			"dedupe_key":          stringSchema("Optional caller-provided dedupe key."),
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("schedule schema encode: %v", err))
	}
	return body
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func stringArraySchema(description string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": description}
}
