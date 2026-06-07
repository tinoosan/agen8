package task

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		return requestInput{}, fmt.Errorf("task: invalid arguments: %w", err)
	}
	action := strings.TrimSpace(strings.ToLower(raw.Action))
	if !containsAction(action) {
		return requestInput{}, fmt.Errorf("task: unsupported action %q", action)
	}
	limit := 20
	if raw.Limit != nil {
		limit = *raw.Limit
	}
	if limit < 0 {
		return requestInput{}, fmt.Errorf("task: limit must be non-negative")
	}
	offset := ptrInt(raw.Offset)
	if offset < 0 {
		return requestInput{}, fmt.Errorf("task: offset must be non-negative")
	}
	metadata, err := decodeMetadata(raw.Metadata)
	if err != nil {
		return requestInput{}, err
	}
	return requestInput{
		Action:             action,
		TaskID:             trimPtr(raw.TaskID),
		AssigneeMemberID:   trimPtr(raw.AssigneeMemberID),
		Status:             strings.TrimSpace(strings.ToLower(trimPtr(raw.Status))),
		Title:              trimPtr(raw.Title),
		Limit:              limit,
		Offset:             offset,
		Metadata:           metadata,
		AcceptanceCriteria: cleanStringSlice(raw.AcceptanceCriteria),
		KeyResultRef:       trimPtr(raw.KeyResultRef),
		MissionRef:         trimPtr(raw.MissionRef),
		Description:        trimPtr(raw.Description),
		TaskKind:           trimPtr(raw.TaskKind),
		Summary:            trimPtr(raw.Summary),
		Artifacts:          cleanStringSlice(raw.Artifacts),
		Reason:             trimPtr(raw.Reason),
		Note:               trimPtr(raw.Note),
		Decision:           strings.TrimSpace(strings.ToLower(trimPtr(raw.Decision))),
		Criteria:           cleanReviewCriteria(raw.Criteria),
	}, nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("task: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("task: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("task: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("task: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("task: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("task: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"create": fieldSet(
		"action",
		"assignee_member_id",
		"title",
		"description",
		"task_kind",
		"metadata",
		"acceptance_criteria",
		"key_result_ref",
		"mission_ref",
	),
	"get":      fieldSet("action", "task_id"),
	"list":     fieldSet("action", "assignee_member_id", "status", "limit", "offset"),
	"update":   fieldSet("action", "task_id", "title", "description", "acceptance_criteria", "task_kind", "key_result_ref", "mission_ref", "metadata"),
	"claim":    fieldSet("action", "task_id"),
	"release":  fieldSet("action", "task_id"),
	"submit":   fieldSet("action", "task_id", "summary", "artifacts", "metadata"),
	"block":    fieldSet("action", "task_id", "reason"),
	"unblock":  fieldSet("action", "task_id", "note"),
	"reassign": fieldSet("action", "task_id", "assignee_member_id"),
	"cancel":   fieldSet("action", "task_id", "reason"),
	"review":   fieldSet("action", "task_id", "decision", "reason", "summary", "note", "criteria"),
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

func decodeMetadata(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("task: metadata must be an object with string values")
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("task: metadata keys must be non-empty")
		}
		out[key] = value
	}
	return out, nil
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

func cleanStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func cleanReviewCriteria(values []reviewCriterionInput) []reviewCriterionInput {
	if len(values) == 0 {
		return nil
	}
	out := make([]reviewCriterionInput, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		out = append(out, value)
	}
	return out
}
