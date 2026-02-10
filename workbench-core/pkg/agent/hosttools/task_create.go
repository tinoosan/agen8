package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// TaskCreateTool creates a DB-backed task.
type TaskCreateTool struct {
	Store           state.TaskStore
	SessionID       string
	RunID           string
	TeamID          string
	RoleName        string
	IsCoordinator   bool
	CoordinatorRole string
	ValidRoles      []string
}

func (t *TaskCreateTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_create",
			Description: "[TASKS] Create a new pending task in SQLite. This is DB-routed (no /inbox file writes).",
			// Keep this tool non-strict: strict mode requires (1) additionalProperties=false
			// for each object and (2) every property to be required, which doesn't fit
			// optional inputs/metadata maps.
			Strict: false,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal": map[string]any{
						"type":        "string",
						"description": "Task goal/instructions",
					},
					"priority": map[string]any{
						"type":        "integer",
						"description": "Priority (0=highest). Default: 5",
					},
					"taskId": map[string]any{
						"type":        "string",
						"description": "Optional explicit task id (for idempotency/deduping).",
					},
					"inputs": map[string]any{
						"type":                 "object",
						"description":          "Optional structured inputs for the task.",
						"additionalProperties": true,
					},
					"metadata": map[string]any{
						"type":                 "object",
						"description":          "Optional metadata for the task.",
						"additionalProperties": true,
					},
					"assignedRole": map[string]any{
						"type":        "string",
						"description": "Optional role assignment in team mode. Omit to assign to your own role.",
					},
				},
				"required":             []any{"goal"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *TaskCreateTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	if t == nil || t.Store == nil {
		return types.HostOpRequest{}, fmt.Errorf("task_create: store is not configured")
	}

	var payload struct {
		Goal         string         `json:"goal"`
		Priority     *int           `json:"priority"`
		TaskID       string         `json:"taskId"`
		Inputs       map[string]any `json:"inputs"`
		Metadata     map[string]any `json:"metadata"`
		AssignedRole string         `json:"assignedRole"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return types.HostOpRequest{}, err
	}

	goal := strings.TrimSpace(payload.Goal)
	if goal == "" {
		return types.HostOpRequest{}, fmt.Errorf("task_create.goal is required")
	}

	now := time.Now().UTC()
	taskID := strings.TrimSpace(payload.TaskID)
	if taskID == "" {
		taskID = fmt.Sprintf("task-%s-%s", now.Format("20060102T150405Z"), uuid.NewString())
	}

	priority := 5
	if payload.Priority != nil {
		priority = *payload.Priority
	}

	task := types.Task{
		TaskID:         taskID,
		SessionID:      strings.TrimSpace(t.SessionID),
		RunID:          strings.TrimSpace(t.RunID),
		AssignedToType: "agent",
		AssignedTo:     strings.TrimSpace(t.RunID),
		TaskKind:       state.TaskKindTask,
		Goal:           goal,
		Inputs:         payload.Inputs,
		Priority:       priority,
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Metadata:       payload.Metadata,
	}
	if task.Inputs == nil {
		task.Inputs = map[string]any{}
	}
	if task.Metadata == nil {
		task.Metadata = map[string]any{}
	}
	if _, ok := task.Metadata["source"]; !ok {
		task.Metadata["source"] = "task_create"
	}

	assignedRole := strings.TrimSpace(payload.AssignedRole)
	teamID := strings.TrimSpace(t.TeamID)
	if teamID != "" {
		roleName := strings.TrimSpace(t.RoleName)
		if assignedRole == "" {
			assignedRole = roleName
		}
		if err := t.validateAssignedRole(assignedRole); err != nil {
			return types.HostOpRequest{}, err
		}
		task.TeamID = teamID
		task.AssignedRole = assignedRole
		task.AssignedToType = "role"
		task.AssignedTo = assignedRole
		task.CreatedBy = roleName
	}

	if err := t.Store.CreateTask(ctx, task); err != nil {
		// Treat "already exists" as success for idempotency.
		if _, gerr := t.Store.GetTask(ctx, taskID); gerr != nil {
			return types.HostOpRequest{}, err
		}
	}
	return types.HostOpRequest{
		Op:   types.HostOpFSList,
		Path: "/workspace",
	}, nil
}

func (t *TaskCreateTool) validateAssignedRole(assignedRole string) error {
	assignedRole = strings.TrimSpace(assignedRole)
	roleName := strings.TrimSpace(t.RoleName)
	if assignedRole == "" {
		return fmt.Errorf("task_create.assignedRole is required in team mode")
	}
	validRoles := map[string]struct{}{}
	for _, role := range t.ValidRoles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		validRoles[role] = struct{}{}
	}
	if len(validRoles) != 0 {
		if _, ok := validRoles[assignedRole]; !ok {
			return fmt.Errorf("task_create.assignedRole %q is not a valid team role", assignedRole)
		}
	}
	if t.IsCoordinator {
		return nil
	}
	if assignedRole == roleName {
		return nil
	}
	if assignedRole == strings.TrimSpace(t.CoordinatorRole) {
		return nil
	}
	return fmt.Errorf("task_create.assignedRole %q is not permitted for role %q", assignedRole, roleName)
}
