package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// TaskCreateTool creates a DB-backed task and also writes an /inbox JSON envelope.
//
// SQLite is the source of truth for tasks; the /inbox file acts as a durable trigger
// and provides parity with external integrations that enqueue via VFS writes.
type TaskCreateTool struct {
	Store     state.TaskStore
	SessionID string
	RunID     string
	InboxPath string
}

func (t *TaskCreateTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_create",
			Description: "[TASKS] Create a new pending task (persist to SQLite) and enqueue it via /inbox so the autonomous loop will pick it up.",
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
		Goal     string         `json:"goal"`
		Priority *int           `json:"priority"`
		TaskID   string         `json:"taskId"`
		Inputs   map[string]any `json:"inputs"`
		Metadata map[string]any `json:"metadata"`
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
		TaskID:    taskID,
		SessionID: strings.TrimSpace(t.SessionID),
		RunID:     strings.TrimSpace(t.RunID),
		Goal:      goal,
		Inputs:    payload.Inputs,
		Priority:  priority,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  payload.Metadata,
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

	if err := t.Store.CreateTask(ctx, task); err != nil {
		// Treat "already exists" as success for idempotency.
		if _, gerr := t.Store.GetTask(ctx, taskID); gerr != nil {
			return types.HostOpRequest{}, err
		}
	}

	inbox := strings.TrimRight(strings.TrimSpace(t.InboxPath), "/")
	if inbox == "" {
		inbox = "/inbox"
	}
	inboxPath := path.Join(inbox, taskID+".json")

	b, _ := json.MarshalIndent(task, "", "  ")
	return types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: resolveVFSPath(inboxPath),
		Text: string(b),
	}, nil
}
