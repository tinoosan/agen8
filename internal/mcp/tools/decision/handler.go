package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
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
	if call.Decisions == nil {
		return Result{}, fmt.Errorf("decision: decision service is not configured")
	}
	switch input.Action {
	case "log":
		return h.log(ctx, call, input)
	case "delete":
		return h.delete(ctx, call, input)
	default:
		return Result{}, fmt.Errorf("decision: unsupported action %q", input.Action)
	}
}

func (h Handler) log(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	projectID, err := requireString(call.ProjectID, "project_id")
	if err != nil {
		return Result{}, err
	}
	memberID, err := requireString(call.ActorMemberID, "member_id")
	if err != nil {
		return Result{}, err
	}
	title, err := requireString(input.Title, "title")
	if err != nil {
		return Result{}, err
	}
	rationale, err := requireString(input.Rationale, "rationale")
	if err != nil {
		return Result{}, err
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:    strings.TrimSpace(call.UserID),
		MemberID:  memberID,
		ProjectID: types.ProjectID(projectID),
	})
	result, err := call.Decisions.Log(ctx, decisionapp.LogRequest{
		ProjectID:              projectID,
		MemberID:               memberID,
		Title:                  title,
		Rationale:              rationale,
		Context:                input.Context,
		AlternativesRejected:   input.AlternativesRejected,
		InvalidationConditions: input.InvalidationConditions,
		Confidence:             input.Confidence,
		TaskRef:                input.TaskRef,
		KeyResultRef:           input.KeyResultRef,
		MissionRef:             input.MissionRef,
	})
	if err != nil {
		return Result{}, err
	}
	return resultFromStructured(map[string]any{
		"ok":       true,
		"tool":     Name,
		"action":   "log",
		"decision": resultEntry(result),
	})
}

func (h Handler) delete(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	projectID, err := requireString(call.ProjectID, "project_id")
	if err != nil {
		return Result{}, err
	}
	memberID, err := requireString(call.ActorMemberID, "member_id")
	if err != nil {
		return Result{}, err
	}
	decisionID, err := requireString(input.DecisionID, "decision_id")
	if err != nil {
		return Result{}, err
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:    strings.TrimSpace(call.UserID),
		MemberID:  memberID,
		ProjectID: types.ProjectID(projectID),
	})
	if err := call.Decisions.Delete(ctx, decisiondomain.DecisionID(decisionID)); err != nil {
		return Result{}, err
	}
	return resultFromStructured(map[string]any{
		"ok":      true,
		"tool":    Name,
		"action":  "delete",
		"deleted": map[string]any{"id": decisionID},
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
		return requestInput{}, fmt.Errorf("decision: invalid arguments: %w", err)
	}
	input := requestInput{
		Action:                 strings.TrimSpace(strings.ToLower(raw.Action)),
		InvalidationConditions: compactStrings(raw.InvalidationConditions),
	}
	if input.Action == "" {
		return requestInput{}, fmt.Errorf("decision: action is required")
	}
	if raw.DecisionID != nil {
		input.DecisionID = strings.TrimSpace(*raw.DecisionID)
	}
	if raw.Title != nil {
		input.Title = strings.TrimSpace(*raw.Title)
	}
	if raw.Rationale != nil {
		input.Rationale = strings.TrimSpace(*raw.Rationale)
	}
	if raw.Context != nil {
		input.Context = strings.TrimSpace(*raw.Context)
	}
	if raw.AlternativesRejected != nil {
		input.AlternativesRejected = strings.TrimSpace(*raw.AlternativesRejected)
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
	return input, nil
}

func validateActionFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("decision: invalid arguments: %w", err)
	}
	actionRaw, ok := fields["action"]
	if !ok || isJSONNull(actionRaw) {
		return fmt.Errorf("decision: action is required")
	}
	var action string
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return fmt.Errorf("decision: action must be a string")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return fmt.Errorf("decision: action is required")
	}
	allowed, ok := fieldsByAction[action]
	if !ok {
		return fmt.Errorf("decision: unsupported action %q", action)
	}
	for field, raw := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("decision: field %q is not valid for action %q", field, action)
		}
		if isJSONNull(raw) {
			return fmt.Errorf("decision: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

var fieldsByAction = map[string]map[string]struct{}{
	"log": fieldSet(
		"action",
		"title",
		"rationale",
		"context",
		"alternatives_rejected",
		"invalidation_conditions",
		"confidence",
		"task_ref",
		"key_result_ref",
		"mission_ref",
	),
	"delete": fieldSet(
		"action",
		"decision_id",
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
		return "", fmt.Errorf("decision: %s is required", field)
	}
	return value, nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
