package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

// NewTaskInput is the validated input shape for task creation.
type NewTaskInput struct {
	ProjectID          types.ProjectID
	CreatedBy          string
	AssignedTo         member.ID
	Description        string
	AcceptanceCriteria []string

	Title        string
	KeyResultRef string
	PlanPhaseID  *uuid.UUID
	PlanTodoID   *uuid.UUID
	Metadata     map[string]any
	TaskKind     string
}

// NewTask validates creation input and returns a pending task.
func NewTask(input NewTaskInput, now time.Time) (Task, error) {
	projectID := types.ProjectID(strings.TrimSpace(string(input.ProjectID)))
	if projectID == "" {
		return Task{}, fmt.Errorf("new task: project id is required")
	}
	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		return Task{}, fmt.Errorf("new task: created by is required")
	}
	assignedTo := member.ID(strings.TrimSpace(string(input.AssignedTo)))
	if assignedTo == "" {
		return Task{}, fmt.Errorf("new task: assigned member id is required")
	}
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return Task{}, fmt.Errorf("new task: description is required")
	}

	id, err := NewTaskID()
	if err != nil {
		return Task{}, fmt.Errorf("new task: generate id: %w", err)
	}

	stamped := now.UTC()
	t := Task{
		ID:                 id,
		ProjectID:          projectID,
		AssignedTo:         assignedTo,
		CreatedBy:          createdBy,
		Title:              strings.TrimSpace(input.Title),
		Description:        description,
		AcceptanceCriteria: newAcceptanceCriteria(input.AcceptanceCriteria),
		Status:             TaskStatusPending,
		KeyResultRef:       strings.TrimSpace(input.KeyResultRef),
		PlanPhaseID:        input.PlanPhaseID,
		PlanTodoID:         input.PlanTodoID,
		Metadata:           input.Metadata,
		TaskKind:           strings.TrimSpace(input.TaskKind),
		CreatedAt:          &stamped,
		UpdatedAt:          &stamped,
	}
	return t, nil
}

func newAcceptanceCriteria(values []string) []AcceptanceCriterion {
	values = cleanStringList(values)
	if len(values) == 0 {
		return nil
	}
	criteria := make([]AcceptanceCriterion, 0, len(values))
	for i, value := range values {
		criteria = append(criteria, AcceptanceCriterion{
			ID:   "criterion-" + strconv.Itoa(i+1),
			Text: value,
		})
	}
	return criteria
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
