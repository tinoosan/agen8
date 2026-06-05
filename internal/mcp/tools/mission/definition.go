package mission

import (
	"encoding/json"
	"fmt"
)

const Name = "mission"
const Description = "[STRATEGY] Mission and key-result gateway."

var allActions = []string{
	"create",
	"get",
	"list",
	"update",
	"activate",
	"pause",
	"complete",
	"archive",
	"history",
	"kr_create",
	"kr_get",
	"kr_list",
	"kr_update",
	"kr_assign_project",
	"kr_drop",
	"kr_reopen",
	"kr_progress",
	"kr_history",
	"progress",
}

func (h Handler) Schema() json.RawMessage {
	return mustSchema()
}

func mustSchema() json.RawMessage {
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":           map[string]any{"type": "string", "enum": allActions},
			"mission_id":       stringSchema("Mission id for read, update, lifecycle, KR list, and mission progress actions."),
			"key_result_id":    stringSchema("Key result id for KR read, update, ownership, progress, history, drop, and reopen actions."),
			"project_id":       stringSchema("Project id override for mission list/create and the owning project for kr_assign_project. Defaults to the MCP session project."),
			"title":            stringSchema("Mission or key-result title."),
			"description":      stringSchema("Mission or key-result description."),
			"status":           stringSchema("Mission status filter for list or mission update target."),
			"start_date":       stringSchema("Mission start date in YYYY-MM-DD form."),
			"end_date":         stringSchema("Mission end date in YYYY-MM-DD form."),
			"limit":            integerSchema("List, history, and kr_list limit. Maximum 50."),
			"offset":           integerSchema("List, history, and kr_list offset."),
			"measurement_type": stringSchema("KR measurement type: number, percentage, or boolean."),
			"direction":        stringSchema("KR direction: increase or decrease. Boolean KRs omit this."),
			"unit":             stringSchema("KR unit label."),
			"baseline":         numberSchema("KR baseline."),
			"target_value":     numberSchema("KR target value."),
			"value":            numberSchema("Progress value for kr_progress."),
			"note":             stringSchema("Progress or lifecycle note for supported actions."),
			"expected_version": integerSchema("Optional optimistic concurrency version for kr_progress."),
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("mission schema encode: %v", err))
	}
	return body
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func containsAction(action string) bool {
	for _, allowed := range allActions {
		if action == allowed {
			return true
		}
	}
	return false
}
