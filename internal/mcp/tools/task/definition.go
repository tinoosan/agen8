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
	"update",
	"claim",
	"release",
	"submit",
	"block",
	"unblock",
	"reassign",
	"cancel",
	"review",
	"attach",
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
			"summary":             stringSchema("Submit summary, or optional review summary for action=review (approve/review)."),
			"artifacts":           stringArraySchema("Artifact refs produced by the worker. Reference files that already exist in the project as file:<vpath> (e.g. file:/project/web/src/App.tsx) — the web viewer opens them and can diff uncommitted changes against git HEAD. Use action=attach only for material that is NOT already in the project tree."),
			"reason":              stringSchema("Reason for block, cancel, retry, or fail review."),
			"note":                stringSchema("Unblock note, or optional review note for any review action."),
			"decision":            stringSchema("Review decision: approve, retry, or fail."),
			"criteria":            reviewCriteriaArraySchema("Acceptance criteria checks for review."),
			"file_name":           stringSchema("Attachment file name for action=attach. A bare name like build-screenshot.png; path separators are rejected. Optional with file_path (defaults to its base name). The file is stored under /project/.agen8/attachments/<task-id>/ and appended to the task's artifacts as file:<vpath>. Attach is for material that does not already live in the project (screenshots, generated reports); a file already in the project tree should be referenced in artifacts as file:<its real vpath> instead of uploading a copy."),
			"content":             stringSchema("Attachment text content for action=attach. Provide exactly one of content, content_b64, or file_path."),
			"content_b64":         stringSchema("Attachment binary content for action=attach, base64-encoded. Provide exactly one of content, content_b64, or file_path. For a binary file that already exists on disk (a screenshot, an image), prefer file_path — re-emitting base64 through the model is slow and corrupts easily."),
			"file_path":           stringSchema("Absolute path to a local file for action=attach. The daemon reads the bytes directly so they never pass through the model — use this for screenshots and other binary evidence. The source file is copied, never moved (max 25MB); file_name defaults to its base name."),
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
