package rpc

import (
	"context"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type Handler struct {
	svc *taskapp.Service
}

func NewHandler(svc *taskapp.Service) *Handler {
	if svc == nil {
		panic("task RPC handler requires task service")
	}
	return &Handler{svc: svc}
}

func (h *Handler) Create(ctx context.Context, p TaskCreateParams) (TaskCreateResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	if spaceID == "" {
		return TaskCreateResult{}, invalidParams("spaceId is required")
	}
	assignedTo := strings.TrimSpace(p.AssignedTo)
	if assignedTo == "" {
		return TaskCreateResult{}, invalidParams("assignedTo is required")
	}
	description := strings.TrimSpace(p.Description)
	if description == "" {
		return TaskCreateResult{}, invalidParams("description is required")
	}
	phaseID, err := parseOptionalUUID(p.PlanPhaseID, "planPhaseId")
	if err != nil {
		return TaskCreateResult{}, err
	}
	todoID, err := parseOptionalUUID(p.PlanTodoID, "planTodoId")
	if err != nil {
		return TaskCreateResult{}, err
	}

	task, err := h.svc.Create(ctx, taskapp.CreateTaskParams{
		SpaceID:            spacedomain.SpaceID(spaceID),
		AssignedTo:         member.ID(assignedTo),
		Description:        description,
		AcceptanceCriteria: append([]string(nil), p.AcceptanceCriteria...),
		Title:              strings.TrimSpace(p.Title),
		KeyResultRef:       strings.TrimSpace(p.KeyResultRef),
		MissionRef:         strings.TrimSpace(p.MissionRef),
		PlanPhaseID:        phaseID,
		PlanTodoID:         todoID,
		Metadata:           cloneRequestMetadata(p.Metadata),
		TaskKind:           strings.TrimSpace(p.TaskKind),
	})
	if err != nil {
		return TaskCreateResult{}, internalError("create task", err)
	}
	return TaskCreateResult{Task: NewTaskView(task)}, nil
}

func (h *Handler) Get(ctx context.Context, p TaskGetParams) (TaskGetResult, error) {
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return TaskGetResult{}, invalidParams("taskId is required")
	}
	task, err := h.svc.Get(ctx, domain.TaskID(taskID))
	if err != nil {
		return TaskGetResult{}, internalError("get task", err)
	}
	return TaskGetResult{Task: NewTaskView(task)}, nil
}

func (h *Handler) List(ctx context.Context, p TaskListParams) (TaskListResult, error) {
	if p.Limit < 0 {
		return TaskListResult{}, invalidParams("limit must be non-negative")
	}
	if p.Offset < 0 {
		return TaskListResult{}, invalidParams("offset must be non-negative")
	}
	statuses := make([]domain.TaskStatus, 0, len(p.Status))
	for _, raw := range p.Status {
		status := domain.TaskStatus(strings.TrimSpace(raw))
		if status == "" {
			continue
		}
		statuses = append(statuses, status)
	}
	phaseID, err := parseOptionalUUID(p.PlanPhaseID, "planPhaseId")
	if err != nil {
		return TaskListResult{}, err
	}
	todoID, err := parseOptionalUUID(p.PlanTodoID, "planTodoId")
	if err != nil {
		return TaskListResult{}, err
	}
	filter := domain.TaskFilter{
		SpaceID:     spacedomain.SpaceID(strings.TrimSpace(p.SpaceID)),
		AssignedTo:  member.ID(strings.TrimSpace(p.AssignedTo)),
		ClaimedBy:   member.ID(strings.TrimSpace(p.ClaimedBy)),
		TaskKind:    strings.TrimSpace(p.TaskKind),
		Status:      statuses,
		PlanPhaseID: phaseID,
		PlanTodoID:  todoID,
		Limit:       p.Limit,
		Offset:      p.Offset,
		SortBy:      strings.TrimSpace(p.SortBy),
		SortDesc:    p.SortDesc,
	}

	tasks, err := h.svc.List(ctx, filter)
	if err != nil {
		return TaskListResult{}, internalError("list tasks", err)
	}
	count, err := h.svc.Count(ctx, filter)
	if err != nil {
		return TaskListResult{}, internalError("count tasks", err)
	}
	views := make([]TaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, NewTaskView(task))
	}
	return TaskListResult{Tasks: views, TotalCount: count}, nil
}

func (h *Handler) Update(ctx context.Context, p TaskUpdateParams) (TaskUpdateResult, error) {
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return TaskUpdateResult{}, invalidParams("taskId is required")
	}
	phaseID, err := parseOptionalUUIDPointer(p.PlanPhaseID, "planPhaseId")
	if err != nil {
		return TaskUpdateResult{}, err
	}
	todoID, err := parseOptionalUUIDPointer(p.PlanTodoID, "planTodoId")
	if err != nil {
		return TaskUpdateResult{}, err
	}
	task, err := h.svc.Update(ctx, taskapp.UpdateTaskParams{
		TaskID:             domain.TaskID(taskID),
		Title:              p.Title,
		Description:        p.Description,
		AcceptanceCriteria: p.AcceptanceCriteria,
		TaskKind:           p.TaskKind,
		KeyResultRef:       p.KeyResultRef,
		PlanPhaseID:        phaseID,
		PlanTodoID:         todoID,
		Metadata:           cloneRequestMetadata(p.Metadata),
	})
	if err != nil {
		return TaskUpdateResult{}, internalError("update task", err)
	}
	return TaskUpdateResult{Task: NewTaskView(task)}, nil
}

func (h *Handler) Cancel(ctx context.Context, p TaskCancelParams) (TaskCancelResult, error) {
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return TaskCancelResult{}, invalidParams("taskId is required")
	}
	reason := strings.TrimSpace(p.Reason)
	if reason == "" {
		return TaskCancelResult{}, invalidParams("reason is required")
	}
	task, err := h.svc.Cancel(ctx, domain.TaskID(taskID), reason)
	if err != nil {
		return TaskCancelResult{}, internalError("cancel task", err)
	}
	return TaskCancelResult{Task: NewTaskView(task)}, nil
}

func parseOptionalUUID(raw, field string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil, invalidParams(field + " must be a valid UUID")
	}
	return &parsed, nil
}

func parseOptionalUUIDPointer(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	return parseOptionalUUID(*raw, field)
}

func cloneRequestMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
