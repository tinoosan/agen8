package mission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decode(args json.RawMessage) (requestInput, error) {
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw rawRequest
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("mission: invalid arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return requestInput{}, fmt.Errorf("mission: invalid arguments: trailing JSON")
	}
	action := strings.TrimSpace(strings.ToLower(raw.Action))
	if !containsAction(action) {
		return requestInput{}, fmt.Errorf("mission: unsupported action %q", action)
	}
	limit := 20
	if raw.Limit != nil {
		limit = *raw.Limit
	}
	if limit < 0 {
		return requestInput{}, fmt.Errorf("mission: limit must be non-negative")
	}
	if limit > 50 {
		limit = 50
	}
	offset := ptrInt(raw.Offset)
	if offset < 0 {
		return requestInput{}, fmt.Errorf("mission: offset must be non-negative")
	}
	return requestInput{
		Action:          action,
		MissionID:       trimPtr(raw.MissionID),
		KeyResultID:     trimPtr(raw.KeyResultID),
		ProjectID:       trimPtr(raw.ProjectID),
		Title:           trimPtr(raw.Title),
		Description:     trimPtr(raw.Description),
		Status:          strings.TrimSpace(strings.ToLower(trimPtr(raw.Status))),
		StartDate:       trimPtr(raw.StartDate),
		EndDate:         trimPtr(raw.EndDate),
		Limit:           limit,
		Offset:          offset,
		MeasurementType: strings.TrimSpace(strings.ToLower(trimPtr(raw.MeasurementType))),
		Direction:       strings.TrimSpace(strings.ToLower(trimPtr(raw.Direction))),
		Unit:            trimPtr(raw.Unit),
		Baseline:        raw.Baseline,
		TargetValue:     raw.TargetValue,
		Value:           raw.Value,
		Note:            trimPtr(raw.Note),
		ExpectedVersion: ptrInt64(raw.ExpectedVersion),
	}, nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("mission: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("mission: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("mission: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("mission: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("mission: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("mission: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"create": fieldSet("action", "project_id", "title", "description", "start_date", "end_date"),
	"get":    fieldSet("action", "mission_id"),
	"list":   fieldSet("action", "project_id", "status", "limit", "offset"),
	"update": fieldSet("action", "mission_id", "title", "description", "status", "start_date", "end_date"),
	"activate": fieldSet(
		"action",
		"mission_id",
		"note",
	),
	"pause":    fieldSet("action", "mission_id", "note"),
	"complete": fieldSet("action", "mission_id", "note"),
	"archive":  fieldSet("action", "mission_id", "note"),
	"history":  fieldSet("action", "mission_id", "limit", "offset"),
	"kr_create": fieldSet(
		"action",
		"mission_id",
		"title",
		"description",
		"measurement_type",
		"direction",
		"unit",
		"baseline",
		"target_value",
	),
	"kr_get":          fieldSet("action", "key_result_id"),
	"kr_list":         fieldSet("action", "mission_id", "limit", "offset"),
	"kr_update":       fieldSet("action", "key_result_id", "title", "description", "measurement_type", "direction", "unit", "baseline", "target_value"),
	"kr_assign_project": fieldSet("action", "key_result_id", "project_id"),
	"kr_drop":         fieldSet("action", "key_result_id", "note"),
	"kr_reopen":       fieldSet("action", "key_result_id", "note"),
	"kr_progress":     fieldSet("action", "key_result_id", "value", "note", "expected_version"),
	"kr_history":      fieldSet("action", "key_result_id"),
	"progress":        fieldSet("action", "mission_id"),
}

func fieldSet(fields ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		out[field] = struct{}{}
	}
	return out
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func trimPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ptrInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func ptrInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
