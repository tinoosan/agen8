package schedule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func decode(args json.RawMessage) (requestInput, error) {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw rawRequest
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("schedule: invalid arguments: %w", err)
	}
	if dec.More() {
		return requestInput{}, fmt.Errorf("schedule: invalid arguments: trailing JSON")
	}
	input := requestInput{
		Action:             strings.TrimSpace(raw.Action),
		ScheduleID:         trimPtr(raw.ScheduleID),
		Title:              trimPtr(raw.Title),
		Description:        trimPtr(raw.Description),
		Mode:               trimPtr(raw.Mode),
		RunAt:              trimPtr(raw.RunAt),
		Cron:               trimPtr(raw.Cron),
		Timezone:           trimPtr(raw.Timezone),
		TargetKind:         trimPtr(raw.TargetKind),
		TargetMemberID:     trimPtr(raw.TargetMemberID),
		TaskTitle:          trimPtr(raw.TaskTitle),
		TaskDescription:    trimPtr(raw.TaskDescription),
		AcceptanceCriteria: raw.AcceptanceCriteria,
		MissionRef:         trimPtr(raw.MissionRef),
		KeyResultRef:       trimPtr(raw.KeyResultRef),
		Status:             trimPtr(raw.Status),
		ExpiresAt:          trimPtr(raw.ExpiresAt),
		DedupeKey:          trimPtr(raw.DedupeKey),
	}
	if raw.IntervalSeconds != nil {
		input.IntervalSeconds = *raw.IntervalSeconds
	}
	if raw.Limit != nil {
		input.Limit = *raw.Limit
	}
	if input.Action == "" {
		return requestInput{}, fmt.Errorf("schedule: action is required")
	}
	if !containsAction(input.Action) {
		return requestInput{}, fmt.Errorf("schedule: unsupported action %q", input.Action)
	}
	if input.Limit < 0 {
		return requestInput{}, fmt.Errorf("schedule: limit must be non-negative")
	}
	if input.Limit > 50 {
		return requestInput{}, fmt.Errorf("schedule: limit must be <= 50")
	}
	return input, nil
}

func validateActionFields(args json.RawMessage) error {
	raw, err := rawMap(args)
	if err != nil {
		return fmt.Errorf("schedule: invalid arguments: %w", err)
	}
	actionRaw, ok := raw["action"]
	if !ok || len(bytes.TrimSpace(actionRaw)) == 0 {
		return fmt.Errorf("schedule: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("schedule: action must be a string")
	}
	action = strings.TrimSpace(action)
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("schedule: unsupported action %q", action)
	}
	for field, value := range raw {
		if !allowed[field] {
			return fmt.Errorf("schedule: field %q is not valid for action %q", field, action)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("schedule: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]bool{
	"create": fieldSet("action", "title", "description", "mode", "run_at", "interval_seconds", "cron", "timezone", "target_kind", "target_member_id", "task_title", "task_description", "acceptance_criteria", "mission_ref", "key_result_ref", "expires_at", "dedupe_key"),
	"get":    fieldSet("action", "schedule_id"),
	"list":   fieldSet("action", "status", "limit"),
	"update": fieldSet("action", "schedule_id", "title", "description", "mode", "run_at", "interval_seconds", "cron", "timezone", "target_kind", "target_member_id", "task_title", "task_description", "acceptance_criteria", "mission_ref", "key_result_ref", "expires_at", "dedupe_key"),
	"cancel": fieldSet("action", "schedule_id"),
}

func fieldSet(fields ...string) map[string]bool {
	out := make(map[string]bool, len(fields))
	for _, field := range fields {
		out[field] = true
	}
	return out
}

func containsAction(action string) bool {
	for _, allowed := range allActions {
		if action == allowed {
			return true
		}
	}
	return false
}

func trimPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
