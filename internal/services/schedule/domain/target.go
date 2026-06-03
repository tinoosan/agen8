package domain

import (
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type TargetKind string

const TargetKindTaskCreate TargetKind = "task.create"

type Target struct {
	Kind       TargetKind        `json:"kind"`
	TaskCreate TaskCreatePayload `json:"taskCreate,omitempty"`
}

type TaskCreatePayload struct {
	TargetMemberID     member.ID `json:"targetMemberId"`
	Title              string    `json:"title"`
	Description        string    `json:"description,omitempty"`
	AcceptanceCriteria []string  `json:"acceptanceCriteria,omitempty"`
	Priority           int       `json:"priority,omitempty"`
	MissionID          string    `json:"missionId,omitempty"`
	KeyResultID        string    `json:"keyResultId,omitempty"`
	PlanTodoID         string    `json:"planTodoId,omitempty"`
}

func (t Target) Validate() error {
	switch t.Kind {
	case TargetKindTaskCreate:
		return t.TaskCreate.Validate()
	default:
		return fmt.Errorf("invalid schedule target kind %q", t.Kind)
	}
}

func (t Target) normalized() Target {
	next := t
	if next.Kind == TargetKindTaskCreate {
		next.TaskCreate = next.TaskCreate.normalized()
	}
	return next
}

func (t Target) NormalizedForUpdate() Target {
	return t.normalized()
}

func (p TaskCreatePayload) Validate() error {
	if strings.TrimSpace(string(p.TargetMemberID)) == "" {
		return fmt.Errorf("targetMemberId is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("task title is required")
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("task description is required")
	}
	return nil
}

func (p TaskCreatePayload) normalized() TaskCreatePayload {
	next := p
	next.TargetMemberID = member.ID(strings.TrimSpace(string(next.TargetMemberID)))
	next.Title = strings.TrimSpace(next.Title)
	next.Description = strings.TrimSpace(next.Description)
	next.MissionID = strings.TrimSpace(next.MissionID)
	next.KeyResultID = strings.TrimSpace(next.KeyResultID)
	next.PlanTodoID = strings.TrimSpace(next.PlanTodoID)
	next.AcceptanceCriteria = trimStrings(next.AcceptanceCriteria)
	return next
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
