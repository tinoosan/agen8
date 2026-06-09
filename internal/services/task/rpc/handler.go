package rpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	"github.com/tinoosan/agen8/internal/services/task/domain"
)

type MemberDisplayLookup interface {
	DisplayName(ctx context.Context, id member.ID) (string, error)
}

type Handler struct {
	svc     *taskapp.Service
	members MemberDisplayLookup
}

func NewHandler(svc *taskapp.Service, members MemberDisplayLookup) *Handler {
	if svc == nil {
		panic("task RPC handler requires task service")
	}
	return &Handler{svc: svc, members: members}
}

func (h *Handler) Create(ctx context.Context, p TaskCreateParams) (TaskCreateResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return TaskCreateResult{}, invalidParams("projectId is required")
	}
	assignedTo := strings.TrimSpace(p.AssignedTo)
	if assignedTo == "" {
		return TaskCreateResult{}, invalidParams("assignedTo is required")
	}
	description := strings.TrimSpace(p.Description)
	if description == "" {
		return TaskCreateResult{}, invalidParams("description is required")
	}

	task, err := h.svc.Create(ctx, taskapp.CreateTaskParams{
		ProjectID:          types.ProjectID(projectID),
		AssignedTo:         member.ID(assignedTo),
		Description:        description,
		AcceptanceCriteria: append([]string(nil), p.AcceptanceCriteria...),
		Title:              strings.TrimSpace(p.Title),
		KeyResultRef:       strings.TrimSpace(p.KeyResultRef),
		MissionRef:         strings.TrimSpace(p.MissionRef),
		Metadata:           cloneRequestMetadata(p.Metadata),
		TaskKind:           strings.TrimSpace(p.TaskKind),
	})
	if err != nil {
		return TaskCreateResult{}, internalError("create task", err)
	}
	return TaskCreateResult{Task: h.newTaskView(ctx, task)}, nil
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
	return TaskGetResult{Task: h.newTaskView(ctx, task)}, nil
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
	filter := domain.TaskFilter{
		ProjectID:  types.ProjectID(strings.TrimSpace(p.ProjectID)),
		AssignedTo: member.ID(strings.TrimSpace(p.AssignedTo)),
		ClaimedBy:  member.ID(strings.TrimSpace(p.ClaimedBy)),
		TaskKind:   strings.TrimSpace(p.TaskKind),
		Status:     statuses,
		Limit:      p.Limit,
		Offset:     p.Offset,
		SortBy:     strings.TrimSpace(p.SortBy),
		SortDesc:   p.SortDesc,
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
		views = append(views, h.newTaskView(ctx, task))
	}
	return TaskListResult{Tasks: views, TotalCount: count}, nil
}

func (h *Handler) Update(ctx context.Context, p TaskUpdateParams) (TaskUpdateResult, error) {
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return TaskUpdateResult{}, invalidParams("taskId is required")
	}
	task, err := h.svc.Update(ctx, taskapp.UpdateTaskParams{
		TaskID:             domain.TaskID(taskID),
		Title:              p.Title,
		Description:        p.Description,
		AcceptanceCriteria: p.AcceptanceCriteria,
		TaskKind:           p.TaskKind,
		KeyResultRef:       p.KeyResultRef,
		Metadata:           cloneRequestMetadata(p.Metadata),
	})
	if err != nil {
		return TaskUpdateResult{}, internalError("update task", err)
	}
	return TaskUpdateResult{Task: h.newTaskView(ctx, task)}, nil
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
	return TaskCancelResult{Task: h.newTaskView(ctx, task)}, nil
}

func (h *Handler) Assign(ctx context.Context, p TaskAssignParams) (TaskAssignResult, error) {
	taskID := strings.TrimSpace(p.TaskID)
	if taskID == "" {
		return TaskAssignResult{}, invalidParams("taskId is required")
	}
	assignedTo := strings.TrimSpace(p.AssignedTo)
	if assignedTo == "" {
		return TaskAssignResult{}, invalidParams("assignedTo is required")
	}
	task, err := h.svc.Assign(ctx, taskapp.AssignTaskParams{
		TaskID:     domain.TaskID(taskID),
		AssignedTo: member.ID(assignedTo),
	})
	if err != nil {
		return TaskAssignResult{}, internalError("assign task", err)
	}
	return TaskAssignResult{Task: h.newTaskView(ctx, task)}, nil
}

func (h *Handler) newTaskView(ctx context.Context, task domain.Task) TaskView {
	view := NewTaskView(task)
	if view.AssignedToLabel == "" {
		view.AssignedToLabel = h.memberLabel(ctx, task.AssignedTo)
	}
	if view.ClaimedByLabel == "" {
		view.ClaimedByLabel = h.memberLabel(ctx, task.ClaimedByMemberID)
	}
	if view.CreatedByLabel == "" {
		view.CreatedByLabel = h.memberLabel(ctx, member.ID(task.CreatedBy))
	}
	return view
}

func (h *Handler) memberLabel(ctx context.Context, id member.ID) string {
	if h == nil || h.members == nil {
		return ""
	}
	id = member.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return ""
	}
	name, err := h.members.DisplayName(ctx, id)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
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
