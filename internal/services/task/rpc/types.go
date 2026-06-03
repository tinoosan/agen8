package rpc

import (
	"strings"
	"time"

	"github.com/google/uuid"
	domain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

// TaskView is the task service RPC read model.
// It projects the rebuilt task aggregate without carrying old run/role/lineage fields.
type TaskView struct {
	ID                 string                       `json:"id"`
	SpaceID            string                       `json:"spaceId"`
	AssignedTo         string                       `json:"assignedTo,omitempty"`
	ClaimedByMemberID  string                       `json:"claimedByMemberId,omitempty"`
	TaskKind           string                       `json:"taskKind,omitempty"`
	CreatedBy          string                       `json:"createdBy,omitempty"`
	Title              string                       `json:"title,omitempty"`
	Description        string                       `json:"description"`
	AcceptanceCriteria []domain.AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	Status             string                       `json:"status"`
	Summary            string                       `json:"summary,omitempty"`
	Error              string                       `json:"error,omitempty"`
	Artifacts          []string                     `json:"artifacts,omitempty"`
	KeyResultRef       string                       `json:"keyResultRef,omitempty"`
	MissionRef         string                       `json:"missionRef,omitempty"`
	PlanPhaseID        string                       `json:"planPhaseId,omitempty"`
	PlanTodoID         string                       `json:"planTodoId,omitempty"`
	Metadata           map[string]any               `json:"metadata,omitempty"`
	CreatedAt          *time.Time                   `json:"createdAt,omitempty"`
	StartedAt          *time.Time                   `json:"startedAt,omitempty"`
	CompletedAt        *time.Time                   `json:"completedAt,omitempty"`
	UpdatedAt          *time.Time                   `json:"updatedAt,omitempty"`
}

func NewTaskView(task domain.Task) TaskView {
	return TaskView{
		ID:                 string(task.ID),
		SpaceID:            string(task.SpaceID),
		AssignedTo:         string(task.AssignedTo),
		ClaimedByMemberID:  string(task.ClaimedByMemberID),
		TaskKind:           task.TaskKind,
		CreatedBy:          task.CreatedBy,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: append([]domain.AcceptanceCriterion(nil), task.AcceptanceCriteria...),
		Status:             string(task.Status),
		Summary:            task.Summary,
		Error:              task.Error,
		Artifacts:          append([]string(nil), task.Artifacts...),
		KeyResultRef:       task.KeyResultRef,
		MissionRef:         missionRefFromMetadata(task.Metadata),
		PlanPhaseID:        uuidString(task.PlanPhaseID),
		PlanTodoID:         uuidString(task.PlanTodoID),
		Metadata:           cloneMetadata(task.Metadata),
		CreatedAt:          cloneTime(task.CreatedAt),
		StartedAt:          cloneTime(task.StartedAt),
		CompletedAt:        cloneTime(task.CompletedAt),
		UpdatedAt:          cloneTime(task.UpdatedAt),
	}
}

func missionRefFromMetadata(metadata map[string]any) string {
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

type TaskCreateParams struct {
	SpaceID            string         `json:"spaceId"`
	AssignedTo         string         `json:"assignedTo"`
	Description        string         `json:"description"`
	AcceptanceCriteria []string       `json:"acceptanceCriteria,omitempty"`
	Title              string         `json:"title,omitempty"`
	TaskKind           string         `json:"taskKind,omitempty"`
	KeyResultRef       string         `json:"keyResultRef,omitempty"`
	MissionRef         string         `json:"missionRef,omitempty"`
	PlanPhaseID        string         `json:"planPhaseId,omitempty"`
	PlanTodoID         string         `json:"planTodoId,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

type TaskCreateResult struct {
	Task TaskView `json:"task"`
}

type TaskGetParams struct {
	TaskID string `json:"taskId"`
}

type TaskGetResult struct {
	Task TaskView `json:"task"`
}

type TaskListParams struct {
	SpaceID     string   `json:"spaceId,omitempty"`
	AssignedTo  string   `json:"assignedTo,omitempty"`
	ClaimedBy   string   `json:"claimedBy,omitempty"`
	TaskKind    string   `json:"taskKind,omitempty"`
	Status      []string `json:"status,omitempty"`
	PlanPhaseID string   `json:"planPhaseId,omitempty"`
	PlanTodoID  string   `json:"planTodoId,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Offset      int      `json:"offset,omitempty"`
	SortBy      string   `json:"sortBy,omitempty"`
	SortDesc    bool     `json:"sortDesc,omitempty"`
}

type TaskListResult struct {
	Tasks      []TaskView `json:"tasks"`
	TotalCount int        `json:"totalCount"`
}

type TaskUpdateParams struct {
	TaskID             string                        `json:"taskId"`
	Title              *string                       `json:"title,omitempty"`
	Description        *string                       `json:"description,omitempty"`
	AcceptanceCriteria *[]domain.AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	TaskKind           *string                       `json:"taskKind,omitempty"`
	KeyResultRef       *string                       `json:"keyResultRef,omitempty"`
	PlanPhaseID        *string                       `json:"planPhaseId,omitempty"`
	PlanTodoID         *string                       `json:"planTodoId,omitempty"`
	Metadata           map[string]any                `json:"metadata,omitempty"`
}

type TaskUpdateResult struct {
	Task TaskView `json:"task"`
}

func uuidString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
