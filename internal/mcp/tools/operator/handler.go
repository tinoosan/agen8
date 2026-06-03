package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	if call.Operator == nil {
		return Result{}, fmt.Errorf("operator: service is not configured")
	}
	projectID, err := requireString(call.ProjectID, "project_id")
	if err != nil {
		return Result{}, err
	}
	memberID, err := requireString(call.ActorMemberID, "member_id")
	if err != nil {
		return Result{}, err
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		MemberID: member.ID(memberID),
		SpaceID:  spacedomain.SpaceID(strings.TrimSpace(call.SpaceID)),
	})

	switch input.Action {
	case "request":
		return h.request(ctx, call, projectID, memberID, input)
	case "escalate":
		return h.escalate(ctx, call, projectID, memberID, input)
	default:
		return Result{}, fmt.Errorf("operator: unsupported action %q", input.Action)
	}
}

func (h Handler) request(ctx context.Context, call CallContext, projectID, memberID string, input requestInput) (Result, error) {
	title, err := requireString(input.Title, "title")
	if err != nil {
		return Result{}, err
	}
	description, err := requireString(input.Description, "description")
	if err != nil {
		return Result{}, err
	}
	category, err := requireString(input.Category, "category")
	if err != nil {
		return Result{}, err
	}
	urgency, err := requireString(input.Urgency, "urgency")
	if err != nil {
		return Result{}, err
	}
	action, err := call.Operator.Create(ctx, operatordomain.CreateParams{
		ProjectID:            projectID,
		SpaceID:              strings.TrimSpace(call.SpaceID),
		TaskRef:              input.TaskRef,
		KeyResultRef:         input.KeyResultRef,
		MissionRef:           input.MissionRef,
		RunID:                input.RunID,
		Blocking:             input.Blocking,
		Source:               operatordomain.OASourceMember,
		MemberID:             memberID,
		Category:             operatordomain.Category(category),
		Urgency:              operatordomain.Urgency(urgency),
		Title:                title,
		Description:          description,
		RequiresVerification: input.RequiresVerification,
		Deadline:             deadlineFromHours(input.DeadlineHours),
		Metadata:             input.Metadata,
	})
	if err != nil {
		return Result{}, err
	}
	return resultFromStructured(map[string]any{
		"ok":       true,
		"tool":     Name,
		"action":   "request",
		"operator": actionEntry(action),
	})
}

func (h Handler) escalate(ctx context.Context, call CallContext, projectID, memberID string, input requestInput) (Result, error) {
	title, err := requireString(input.Title, "title")
	if err != nil {
		return Result{}, err
	}
	description, err := requireString(input.Description, "description")
	if err != nil {
		return Result{}, err
	}
	category, err := requireString(input.Category, "category")
	if err != nil {
		return Result{}, err
	}
	urgency, err := requireString(input.Urgency, "urgency")
	if err != nil {
		return Result{}, err
	}
	recommendation, err := requireString(input.Recommendation, "recommendation")
	if err != nil {
		return Result{}, err
	}
	escalation, err := call.Operator.CreateEscalation(ctx, operatorapp.CreateEscalationParams{
		ProjectID:      projectID,
		SpaceID:        strings.TrimSpace(call.SpaceID),
		TaskRef:        input.TaskRef,
		KeyResultRef:   input.KeyResultRef,
		MissionRef:     input.MissionRef,
		Source:         string(operatordomain.SourceMember),
		MemberID:       memberID,
		Category:       operatordomain.Category(category),
		Urgency:        operatordomain.Urgency(urgency),
		Title:          title,
		Description:    description,
		Recommendation: recommendation,
		Confidence:     input.Confidence,
		Deadline:       deadlineFromHours(input.DeadlineHours),
		Metadata:       input.Metadata,
	})
	if err != nil {
		return Result{}, err
	}
	return resultFromStructured(map[string]any{
		"ok":         true,
		"tool":       Name,
		"action":     "escalate",
		"escalation": escalationEntry(escalation),
	})
}

func decode(args json.RawMessage) (requestInput, error) {
	if err := validateActionFields(args); err != nil {
		return requestInput{}, err
	}
	var raw rawRequest
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("operator: invalid arguments: %w", err)
	}
	input := requestInput{
		Action: strings.TrimSpace(strings.ToLower(raw.Action)),
	}
	metadata, err := decodeMetadata(raw.Metadata)
	if err != nil {
		return requestInput{}, err
	}
	input.Metadata = metadata
	if input.Action == "" {
		return requestInput{}, fmt.Errorf("operator: action is required")
	}
	if raw.Title != nil {
		input.Title = strings.TrimSpace(*raw.Title)
	}
	if raw.Description != nil {
		input.Description = strings.TrimSpace(*raw.Description)
	}
	if raw.Recommendation != nil {
		input.Recommendation = strings.TrimSpace(*raw.Recommendation)
	}
	if raw.Category != nil {
		input.Category = strings.TrimSpace(*raw.Category)
	}
	if raw.Urgency != nil {
		input.Urgency = strings.TrimSpace(*raw.Urgency)
	}
	if raw.Confidence != nil {
		input.Confidence = *raw.Confidence
	}
	if raw.TaskRef != nil {
		input.TaskRef = strings.TrimSpace(*raw.TaskRef)
	}
	if raw.KeyResultRef != nil {
		input.KeyResultRef = strings.TrimSpace(*raw.KeyResultRef)
	}
	if raw.MissionRef != nil {
		input.MissionRef = strings.TrimSpace(*raw.MissionRef)
	}
	if raw.RunID != nil {
		input.RunID = strings.TrimSpace(*raw.RunID)
	}
	if raw.Blocking != nil {
		input.Blocking = *raw.Blocking
	}
	if raw.RequiresVerification != nil {
		input.RequiresVerification = *raw.RequiresVerification
	}
	if raw.DeadlineHours != nil {
		input.DeadlineHours = *raw.DeadlineHours
	}
	return input, nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("operator: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("operator: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("operator: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("operator: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("operator: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("operator: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"request": fieldSet(
		"action",
		"title",
		"description",
		"category",
		"urgency",
		"task_ref",
		"key_result_ref",
		"mission_ref",
		"run_id",
		"blocking",
		"requires_verification",
		"deadline_hours",
		"metadata",
	),
	"escalate": fieldSet(
		"action",
		"title",
		"description",
		"recommendation",
		"category",
		"urgency",
		"confidence",
		"task_ref",
		"key_result_ref",
		"mission_ref",
		"deadline_hours",
		"metadata",
	),
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

func requireString(value, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("operator: %s is required", field)
	}
	return value, nil
}

func deadlineFromHours(hours int) *time.Time {
	if hours <= 0 {
		return nil
	}
	deadline := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
	return &deadline
}

func decodeMetadata(raw json.RawMessage) (map[string]string, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("operator: metadata must be an object with string values")
	}
	out := make(map[string]string, len(values))
	for key, encoded := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		var value string
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("operator: metadata value for %q must be a string", key)
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, nil
}
