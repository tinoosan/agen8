package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type taskEntry struct {
	ID                 string                           `json:"id"`
	ProjectID          string                           `json:"projectId,omitempty"`
	Status             string                           `json:"status,omitempty"`
	Title              string                           `json:"title,omitempty"`
	Description        string                           `json:"description,omitempty"`
	AssignedToMemberID string                           `json:"assignedToMemberId,omitempty"`
	AssignedToLabel    string                           `json:"assignedToLabel,omitempty"`
	ClaimedByMemberID  string                           `json:"claimedByMemberId,omitempty"`
	ClaimedByLabel     string                           `json:"claimedByMemberLabel,omitempty"`
	CreatedBy          string                           `json:"createdBy,omitempty"`
	CreatedByLabel     string                           `json:"createdByLabel,omitempty"`
	KeyResultRef       string                           `json:"keyResultRef,omitempty"`
	MissionRef         string                           `json:"missionRef,omitempty"`
	TaskKind           string                           `json:"taskKind,omitempty"`
	AcceptanceCriteria []taskdomain.AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	Summary            string                           `json:"summary,omitempty"`
	Artifacts          []string                         `json:"artifacts,omitempty"`
	StatusReason       string                           `json:"statusReason,omitempty"`
	Metadata           map[string]any                   `json:"metadata,omitempty"`
}

func (h Handler) taskResult(ctx context.Context, call CallContext, action string, task taskdomain.Task, err error, extra map[string]any) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	task = h.resolveTaskMemberLabels(ctx, call, task)
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": action,
		"task":   toTaskEntry(task),
	}
	for key, value := range extra {
		structured[key] = value
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) taskResultForActor(ctx context.Context, call CallContext, action string, task taskdomain.Task, err error, extra map[string]any, actor actor) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	nextAction, guidance := taskResponseGuidance(action, task, actor.MemberID)
	if nextAction != "" || guidance != "" {
		if extra == nil {
			extra = map[string]any{}
		}
		if nextAction != "" {
			extra["nextAction"] = nextAction
		}
		if guidance != "" {
			extra["guidance"] = guidance
		}
	}
	return h.taskResult(ctx, call, action, task, nil, extra)
}

func taskResponseGuidance(action string, task taskdomain.Task, actorID member.ID) (string, string) {
	actorID = member.ID(strings.TrimSpace(string(actorID)))
	if actorID == "" {
		return "", ""
	}
	switch action {
	case "create", "reassign":
		if task.Status == taskdomain.TaskStatusPending && task.AssignedTo == actorID {
			return "claim", "Claim the task before starting work. Fetch the task when you need the full description and acceptance criteria."
		}
	case "submit":
		if task.Status == taskdomain.TaskStatusInReview && member.ID(strings.TrimSpace(task.CreatedBy)) == actorID {
			return "review", "Fetch the task, inspect the submitted work against the acceptance criteria, then approve, retry, or fail the review. Include a note with your reasoning — what you verified, residual risk, anything notable — it is recorded on the task even when you approve."
		}
	}
	return "", ""
}

func (h Handler) listResult(ctx context.Context, call CallContext, tasks []taskdomain.Task, err error, input requestInput) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	rows := make([]taskEntry, 0, len(tasks))
	for _, task := range tasks {
		task = h.resolveTaskMemberLabels(ctx, call, task)
		rows = append(rows, toTaskEntry(task))
	}
	structured := map[string]any{
		"ok":     true,
		"tool":   Name,
		"action": "list",
		"tasks":  rows,
		"count":  len(rows),
		"limit":  input.Limit,
		"offset": input.Offset,
	}
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

func (h Handler) resolveTaskMemberLabels(ctx context.Context, call CallContext, task taskdomain.Task) taskdomain.Task {
	if call.Members == nil {
		return task
	}
	if strings.TrimSpace(task.AssignedToLabel) == "" {
		task.AssignedToLabel = h.resolveMemberLabel(ctx, call, task.AssignedTo)
	}
	if strings.TrimSpace(task.ClaimedByMemberLabel) == "" {
		task.ClaimedByMemberLabel = h.resolveMemberLabel(ctx, call, task.ClaimedByMemberID)
	}
	if strings.TrimSpace(task.CreatedByLabel) == "" {
		task.CreatedByLabel = h.resolveMemberLabel(ctx, call, member.ID(task.CreatedBy))
	}
	return task
}

func (h Handler) resolveMemberLabel(ctx context.Context, call CallContext, memberID member.ID) string {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" || call.Members == nil {
		return ""
	}
	rosterMember, err := call.Members.GetMember(ctx, memberID)
	if err != nil {
		return ""
	}
	return memberLabel(rosterMember)
}

func toTaskEntry(task taskdomain.Task) taskEntry {
	entry := taskEntry{
		ID:                 strings.TrimSpace(string(task.ID)),
		ProjectID:          strings.TrimSpace(string(task.ProjectID)),
		Status:             strings.TrimSpace(string(task.Status)),
		Title:              strings.TrimSpace(task.Title),
		Description:        strings.TrimSpace(task.Description),
		AssignedToMemberID: strings.TrimSpace(string(task.AssignedTo)),
		AssignedToLabel:    strings.TrimSpace(task.AssignedToLabel),
		ClaimedByMemberID:  strings.TrimSpace(string(task.ClaimedByMemberID)),
		ClaimedByLabel:     strings.TrimSpace(task.ClaimedByMemberLabel),
		CreatedBy:          strings.TrimSpace(task.CreatedBy),
		CreatedByLabel:     strings.TrimSpace(task.CreatedByLabel),
		KeyResultRef:       strings.TrimSpace(task.KeyResultRef),
		MissionRef:         missionRefFromTaskMetadata(task.Metadata),
		TaskKind:           strings.TrimSpace(task.TaskKind),
		AcceptanceCriteria: append([]taskdomain.AcceptanceCriterion(nil), task.AcceptanceCriteria...),
		Summary:            strings.TrimSpace(task.Summary),
		Artifacts:          append([]string(nil), task.Artifacts...),
		StatusReason:       strings.TrimSpace(task.Error),
		Metadata:           task.Metadata,
	}
	return entry
}

func missionRefFromTaskMetadata(metadata map[string]any) string {
	for _, key := range []string{"missionRef", "mission_ref", "missionId", "mission_id"} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func memberLabel(member member.Record) string {
	if strings.TrimSpace(member.DisplayName) != "" {
		return strings.TrimSpace(member.DisplayName)
	}
	if strings.TrimSpace(member.HarnessKind) != "" {
		return strings.TrimSpace(member.HarnessKind)
	}
	if strings.TrimSpace(member.MemberType) != "" {
		return strings.ReplaceAll(strings.TrimSpace(member.MemberType), "_", " ")
	}
	return ""
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("task: encode structured response: %w", err)
	}
	return string(encoded), nil
}
