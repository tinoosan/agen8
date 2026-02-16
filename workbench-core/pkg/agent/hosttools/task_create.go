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

// SpawnWorkerFunc creates a child run for a spawned worker and returns the child RunID.
// The daemon wires this callback to create Run records and add them to the session.
type SpawnWorkerFunc func(ctx context.Context, goal, sessionID, parentRunID string) (childRunID string, err error)

type spawnSignalKey struct{}

// WithSpawnSignal stores a callback in context that is fired each time a worker is successfully spawned.
// The session uses this to detect when delegation has occurred.
func WithSpawnSignal(ctx context.Context, fn func()) context.Context {
	return context.WithValue(ctx, spawnSignalKey{}, fn)
}

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
	// IsChildRun indicates this tool is running in a sub-agent context.
	IsChildRun bool
	// SpawnWorker is called when spawn_worker=true to create a child run.
	// If nil, spawn_worker=true will return an error.
	SpawnWorker SpawnWorkerFunc
}

func (t *TaskCreateTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_create",
			Description: "[TASKS] Create a new pending task in SQLite. You may set spawnWorker=true when you decide to delegate work to a worker agent, or when the user or task goal asks you to use subagents; when the goal requests subagents you MUST set spawnWorker=true and do not do the work yourself. spawnWorker creates an outstanding dependency; do not call final_answer on your coordination task until it is resolved (worker result reviewed via task_review). Do not use sleep or wait—process tasks as they come; the system schedules work. This is DB-routed (no /inbox file writes).",
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
					"spawnWorker": map[string]any{
						"type":        "boolean",
						"description": "Set to true when you decide to delegate to a worker agent, or when the user or goal requests subagents (required when the goal requests subagents). Creates an outstanding dependency; do not call final_answer until it is resolved (worker result reviewed via task_review). Do not sleep or wait.",
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
		SpawnWorker  bool           `json:"spawnWorker"`
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

	if payload.SpawnWorker {
		if t.SpawnWorker == nil {
			switch {
			case t.IsCoordinator:
				return types.HostOpRequest{}, fmt.Errorf("task_create: spawn_worker is not permitted for coordinators; delegate tasks to co-agents instead")
			case t.IsChildRun:
				return types.HostOpRequest{}, fmt.Errorf("task_create: spawn_worker is not permitted for sub-agents; only parent agents can spawn workers")
			default:
				return types.HostOpRequest{}, fmt.Errorf("task_create: spawn_worker is not available in this context")
			}
		}
		childRunID, err := t.SpawnWorker(ctx, goal, t.SessionID, t.RunID)
		if err != nil {
			return types.HostOpRequest{}, fmt.Errorf("task_create: spawn worker: %w", err)
		}
		task.RunID = childRunID
		task.AssignedToType = "agent"
		task.AssignedTo = childRunID
		task.Metadata["source"] = "spawn_worker"
		task.Metadata["parentRunId"] = strings.TrimSpace(t.RunID)
		// Fire spawn signal so the session's delegation detector knows a worker was spawned.
		if fn, ok := ctx.Value(spawnSignalKey{}).(func()); ok && fn != nil {
			fn()
		}
	}

	if err := t.Store.CreateTask(ctx, task); err != nil {
		// Treat "already exists" as success for idempotency.
		if _, gerr := t.Store.GetTask(ctx, taskID); gerr != nil {
			return types.HostOpRequest{}, err
		}
	}
	msg := fmt.Sprintf("Task %s created successfully.", taskID)
	if payload.SpawnWorker {
		msg = fmt.Sprintf("Task %s created and worker agent spawned. You delegated this to a subagent; do not do the same work yourself. When the callback task (callback-%s) appears in your inbox, process it with task_review to verify the work is complete.", taskID, taskID)
		inputForEvent := map[string]string{"goal": goal, "taskId": taskID}
		if task.AssignedToType == "agent" {
			inputForEvent["childRunId"] = task.AssignedTo
		}
		inputJSON, _ := json.Marshal(inputForEvent)
		return types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create", Text: msg, Input: inputJSON}, nil
	}
	inputForEvent := map[string]string{"goal": goal, "taskId": taskID}
	inputJSON, _ := json.Marshal(inputForEvent)
	return types.HostOpRequest{
		Op:    types.HostOpToolResult,
		Tag:   "task_create",
		Text:  msg,
		Input: inputJSON,
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
