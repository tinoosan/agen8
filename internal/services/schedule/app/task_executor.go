package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type TaskCreator interface {
	Create(ctx context.Context, params taskapp.CreateTaskParams) (taskdomain.Task, error)
}

type TaskCreateExecutor struct {
	tasks TaskCreator
}

func NewTaskCreateExecutor(tasks TaskCreator) (*TaskCreateExecutor, error) {
	if tasks == nil {
		return nil, fmt.Errorf("schedule task.create executor: task service is required")
	}
	return &TaskCreateExecutor{tasks: tasks}, nil
}

func (e *TaskCreateExecutor) Execute(ctx context.Context, entry schedule.Entry, run schedule.Run) (TargetResult, error) {
	payload := entry.Target.TaskCreate
	planTodoID, err := optionalUUID(payload.PlanTodoID)
	if err != nil {
		return TargetResult{}, fmt.Errorf("parse plan todo id: %w", err)
	}
	taskCtx := caller.ContextWithCaller(ctx, caller.Caller{
		UserID:   scheduleUserID(entry.CreatedBy),
		MemberID: scheduleMemberID(entry.CreatedBy),
		SpaceID:  entry.SpaceID,
	})
	task, err := e.tasks.Create(taskCtx, taskapp.CreateTaskParams{
		SpaceID:            entry.SpaceID,
		AssignedTo:         payload.TargetMemberID,
		Description:        payload.Description,
		AcceptanceCriteria: payload.AcceptanceCriteria,
		Title:              payload.Title,
		KeyResultRef:       payload.KeyResultID,
		MissionRef:         payload.MissionID,
		PlanTodoID:         planTodoID,
		Metadata: map[string]any{
			"source":          "schedule",
			"scheduleEntryId": string(entry.ID),
			"scheduleRunId":   string(run.ID),
		},
		TaskKind: taskdomain.TaskKindScheduled,
	})
	if err != nil {
		return TargetResult{}, err
	}
	return TargetResult{TargetObjectID: string(task.ID)}, nil
}

func scheduleUserID(actor schedule.ActorRef) string {
	raw := strings.TrimSpace(string(actor))
	if strings.HasPrefix(raw, "member-") {
		return ""
	}
	return raw
}

func scheduleMemberID(actor schedule.ActorRef) member.ID {
	raw := strings.TrimSpace(string(actor))
	if !strings.HasPrefix(raw, "member-") {
		return ""
	}
	return member.ID(raw)
}

func optionalUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
