package task

import (
	"encoding/json"
	"fmt"
)

const Name = "task"
const Description = "[TASKS] Member-routed task lifecycle gateway."

var allActions = []string{
	"create",
	"get",
	"list",
	"claim",
	"release",
	"submit",
	"block",
	"unblock",
	"reassign",
	"cancel",
	"review",
}

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":              map[string]any{"type": "string", "enum": allActions},
			"task_id":             stringSchema("Task id for read or lifecycle actions."),
			"assignee_member_id":  stringSchema("Member id to assign or reassign the task to."),
			"status":              stringSchema("Optional status filter for list."),
			"title":               stringSchema("Short task title for create."),
			"description":         stringSchema("Task description for create."),
			"task_kind":           stringSchema("Optional task kind."),
			"limit":               integerSchema("List limit. Maximum 50."),
			"offset":              integerSchema("List offset."),
			"metadata":            stringMapSchema("Task metadata. Values must be strings."),
			"acceptance_criteria": stringArraySchema("Acceptance criteria for delegated work."),
			"key_result_ref":      stringSchema("Key result this task serves."),
			"mission_ref":         stringSchema("Mission this task serves when no key result is available."),
			"summary":             stringSchema("Submit summary, or optional review summary for action=review."),
			"artifacts":           stringArraySchema("Artifact refs produced by the worker."),
			"reason":              stringSchema("Reason for block, cancel, retry, or fail."),
			"note":                stringSchema("Unblock note, or optional review note for action=review."),
			"decision":            stringSchema("Review decision: approve, retry, or fail."),
			"criteria":            reviewCriteriaArraySchema("Acceptance criteria checks for review."),
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("task schema encode: %v", err))
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

func stringMapSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
		"description":          description,
	}
}

func reviewCriteriaArraySchema(description string) map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"id":        map[string]any{"type": "string"},
				"satisfied": map[string]any{"type": "boolean"},
			},
			"required": []string{"id", "satisfied"},
		},
		"description": description,
	}
}

func containsAction(action string) bool {
	for _, allowed := range allActions {
		if action == allowed {
			return true
		}
	}
	return false
}
