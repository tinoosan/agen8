package task

import (
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
	ClaimedByMemberID  string                           `json:"claimedByMemberId,omitempty"`
	CreatedBy          string                           `json:"createdBy,omitempty"`
	KeyResultRef       string                           `json:"keyResultRef,omitempty"`
	MissionRef         string                           `json:"missionRef,omitempty"`
	TaskKind           string                           `json:"taskKind,omitempty"`
	AcceptanceCriteria []taskdomain.AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	Summary            string                           `json:"summary,omitempty"`
	Artifacts          []string                         `json:"artifacts,omitempty"`
	StatusReason       string                           `json:"statusReason,omitempty"`
	Metadata           map[string]any                   `json:"metadata,omitempty"`
}

func (h Handler) taskResult(action string, task taskdomain.Task, err error, extra map[string]any) (Result, error) {
	if err != nil {
		return Result{}, err
	}
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

func (h Handler) taskResultForActor(action string, task taskdomain.Task, err error, extra map[string]any, actor actor) (Result, error) {
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
	return h.taskResult(action, task, nil, extra)
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
			return "review", "Fetch the task, inspect the submitted work against the acceptance criteria, then approve, retry, or fail the review."
		}
	}
	return "", ""
}

func (h Handler) listResult(tasks []taskdomain.Task, err error, input requestInput) (Result, error) {
	if err != nil {
		return Result{}, err
	}
	rows := make([]taskEntry, 0, len(tasks))
	for _, task := range tasks {
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

func toTaskEntry(task taskdomain.Task) taskEntry {
	entry := taskEntry{
		ID:                 strings.TrimSpace(string(task.ID)),
		ProjectID:          strings.TrimSpace(string(task.ProjectID)),
		Status:             strings.TrimSpace(string(task.Status)),
		Title:              strings.TrimSpace(task.Title),
		Description:        strings.TrimSpace(task.Description),
		AssignedToMemberID: strings.TrimSpace(string(task.AssignedTo)),
		ClaimedByMemberID:  strings.TrimSpace(string(task.ClaimedByMemberID)),
		CreatedBy:          strings.TrimSpace(task.CreatedBy),
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
	return strings.TrimSpace(string(member.ID))
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("task: encode structured response: %w", err)
	}
	return string(encoded), nil
}
