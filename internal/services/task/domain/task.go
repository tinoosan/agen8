package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type TaskID string

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusInReview  TaskStatus = "in_review"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

var validTaskTransitions = map[TaskStatus]map[TaskStatus]bool{
	TaskStatusPending: {
		TaskStatusActive:   true,
		TaskStatusCanceled: true,
	},
	TaskStatusActive: {
		TaskStatusInReview: true,
		TaskStatusBlocked:  true,
		TaskStatusPending:  true,
		TaskStatusCanceled: true,
	},
	TaskStatusBlocked: {
		TaskStatusActive:   true,
		TaskStatusCanceled: true,
	},
	TaskStatusInReview: {
		TaskStatusActive:    true,
		TaskStatusSucceeded: true,
		TaskStatusFailed:    true,
		TaskStatusCanceled:  true,
	},
	TaskStatusSucceeded: {},
	TaskStatusFailed:    {},
	TaskStatusCanceled:  {},
}

func (s TaskStatus) CanTransitionTo(target TaskStatus) bool {
	allowed, exists := validTaskTransitions[s]
	if !exists {
		return false
	}
	return allowed[target]
}

func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCanceled:
		return true
	default:
		return false
	}
}

type Task struct {
	ID                 TaskID                `json:"id"`
	ProjectID          types.ProjectID       `json:"projectId,omitempty"`
	AssignedTo         member.ID             `json:"assignedTo,omitempty"`
	ClaimedByMemberID  member.ID             `json:"claimedByMemberId,omitempty"`
	TaskKind           string                `json:"taskKind,omitempty"`
	CreatedBy          string                `json:"createdBy,omitempty"`
	Title              string                `json:"title,omitempty"`
	Description        string                `json:"description"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptanceCriteria,omitempty"`
	Status             TaskStatus            `json:"status,omitempty"`
	CreatedAt          *time.Time            `json:"createdAt,omitempty"`
	StartedAt          *time.Time            `json:"startedAt,omitempty"`
	CompletedAt        *time.Time            `json:"completedAt,omitempty"`
	UpdatedAt          *time.Time            `json:"updatedAt,omitempty"`
	Error              string                `json:"error,omitempty"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
	Summary            string                `json:"summary,omitempty"`
	Artifacts          []string              `json:"artifacts,omitempty"`
	KeyResultRef       string                `json:"keyResultRef,omitempty"`
}

type AcceptanceCriterion struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Satisfied bool   `json:"satisfied"`
}

func (t Task) SortTime() time.Time {
	if t.CreatedAt != nil {
		return t.CreatedAt.UTC()
	}
	return time.Time{}
}

func (t Task) Lifecycle() types.Lifecycle {
	return types.Lifecycle{
		CreatedAt:   t.CreatedAt,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func (t Task) LifecyclePhase() types.LifecyclePhase {
	return types.LifecyclePhaseForStatus(string(t.Status))
}

func (t *Task) NormalizeStatus() {
	if t == nil {
		return
	}
	status := strings.ToLower(strings.TrimSpace(string(t.Status)))
	if status == "" {
		t.Status = TaskStatusPending
		return
	}
	t.Status = TaskStatus(status)
}

func (t *Task) NormalizeSpaceFields() {
	if t == nil {
		return
	}
	t.ProjectID = types.ProjectID(strings.TrimSpace(string(t.ProjectID)))
}

var ErrUnsafeTaskID = fmt.Errorf("task ID contains path-unsafe characters")

func NewTaskID() (TaskID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate task ID entropy: %w", err)
	}
	return TaskID("task-" + hex.EncodeToString(b[:])), nil
}

func NormalizeTaskID(raw string) (normalized TaskID, changed bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(raw, "task-"):
		return TaskID(raw), false
	case strings.HasPrefix(raw, "heartbeat-"):
		return TaskID(raw), false
	default:
		return TaskID("task-" + raw), true
	}
}

func ValidateTaskID(id string) error {
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return ErrUnsafeTaskID
	}
	return nil
}
