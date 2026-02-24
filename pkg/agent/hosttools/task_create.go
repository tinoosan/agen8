package hosttools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/state"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// SpawnWorkerFunc creates a child run for a spawned worker and returns the child RunID
// plus the canonical child role (for example, "Subagent-1").
// The daemon wires this callback to create Run records and add them to the session.
type SpawnWorkerFunc func(ctx context.Context, goal, sessionID, parentRunID string) (childRunID, childRole string, err error)

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
	if t.SpawnWorker == nil {
		return t.definitionTeamOnly()
	}
	return t.definitionWithSpawnWorker()
}

func (t *TaskCreateTool) assignedRoleDescription() string {
	if t != nil && t.IsCoordinator {
		return "Required in team mode for coordinators. Must be an explicit team role."
	}
	return "Optional role assignment in team mode. Omit to assign to your own role."
}

func (t *TaskCreateTool) teamModeDescription() string {
	if t != nil && t.IsCoordinator {
		return "[TASKS] Create a new pending task in SQLite. In team mode, coordinators must delegate by creating tasks with an explicit assignedRole for each task. This is DB-routed (no /inbox file writes)."
	}
	return "[TASKS] Create a new pending task in SQLite. In team mode, delegate work by creating tasks and assigning them to roles via assignedRole. Omit assignedRole to assign to your own role. This is DB-routed (no /inbox file writes)."
}

// definitionTeamOnly returns the tool definition for team mode (no spawnWorker).
func (t *TaskCreateTool) definitionTeamOnly() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_create",
			Description: t.teamModeDescription(),
			Strict:      false,
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
						"description": t.assignedRoleDescription(),
					},
				},
				"required":             []any{"goal"},
				"additionalProperties": false,
			},
		},
	}
}

// definitionWithSpawnWorker returns the full tool definition including spawnWorker (standalone daemon).
func (t *TaskCreateTool) definitionWithSpawnWorker() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        "task_create",
			Description: "[TASKS] Create a new pending task in SQLite. Use spawnWorker=true when breaking down a large task into smaller tasks (one task per distinct subtask), when you decide to delegate work to a worker, or when the user or goal asks for subagents; when the goal requests subagents you MUST set spawnWorker=true and do not do the work yourself. Examples: research tasks (e.g. research topic X and summarize), comparative analysis (compare A vs B), multi-step investigations (audit then document), parallelizable work (gather from doc X, doc Y). When workers finish, callbacks are created for you to process with task_review. After spawning, do not keep checking for work; callbacks will be provided when workers finish. Do not use sleep or wait—process tasks as they come. This is DB-routed (no /inbox file writes).",
			Strict:      false,
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
						"description": t.assignedRoleDescription(),
					},
					"spawnWorker": map[string]any{
						"type":        "boolean",
						"description": "Set to true when breaking down a large task into smaller tasks (one per subtask), e.g. research, comparative analysis, audits, parallelizable work, or when the user or goal requests subagents (required when the goal requests subagents). In the goal, ask the worker to write outputs under /workspace. After spawning, do not poll or repeatedly check; callbacks will be provided when workers finish. Do not sleep or wait.",
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
	normalizedArgs, err := normalizeTaskCreateArgs(args)
	if err != nil {
		return types.HostOpRequest{}, err
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
	if err := json.Unmarshal(normalizedArgs, &payload); err != nil {
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
	parentTaskID := ParentTaskIDFromContext(ctx)

	assignedRole := strings.TrimSpace(payload.AssignedRole)
	teamID := strings.TrimSpace(t.TeamID)
	if teamID != "" {
		roleName := strings.TrimSpace(t.RoleName)
		if assignedRole == "" && t.IsCoordinator {
			return types.HostOpRequest{}, fmt.Errorf("task_create.assignedRole is required for coordinators in team mode")
		}
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
		if parentTaskID != "" && assignedRole != roleName {
			task.Metadata["batchMode"] = true
			task.Metadata["batchParentTaskId"] = parentTaskID
			task.Metadata["batchWaveId"] = EnsureBatchWaveIDFromContext(ctx, parentTaskID, strings.TrimSpace(t.RunID))
		}
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
		childRunID, childRole, err := t.SpawnWorker(ctx, goal, t.SessionID, t.RunID)
		if err != nil {
			return types.HostOpRequest{}, fmt.Errorf("task_create: spawn worker: %w", err)
		}
		task.RunID = childRunID
		task.AssignedToType = "agent"
		task.AssignedTo = childRunID
		if role := strings.TrimSpace(childRole); role != "" {
			task.AssignedRole = role
		}
		task.Metadata["source"] = "spawn_worker"
		task.Metadata["parentRunId"] = strings.TrimSpace(t.RunID)
		if parentTaskID != "" {
			task.Metadata["batchMode"] = true
			task.Metadata["batchParentTaskId"] = parentTaskID
			task.Metadata["batchWaveId"] = EnsureBatchWaveIDFromContext(ctx, parentTaskID, strings.TrimSpace(t.RunID))
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
		msg = fmt.Sprintf("Task %s created and worker agent spawned. You delegated this to a subagent; do not do the same work yourself. The worker was asked to write outputs under /workspace. When the callback task (callback-%s) appears in your inbox, process it with task_review and inspect worker files under /workspace/subagent-<N>/ and summaries under /tasks/subagent-<N>/<date>/<taskID>/SUMMARY.md.", taskID, taskID)
		inputForEvent := map[string]string{"goal": goal, "taskId": taskID}
		if metadataBool(task.Metadata, "batchMode") {
			inputForEvent["batchMode"] = "true"
			inputForEvent["batchParentTaskId"] = metadataString(task.Metadata, "batchParentTaskId")
			inputForEvent["batchWaveId"] = metadataString(task.Metadata, "batchWaveId")
		}
		if task.AssignedToType == "agent" {
			inputForEvent["childRunId"] = task.AssignedTo
		}
		inputJSON, _ := json.Marshal(inputForEvent)
		return types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create", Text: msg, Input: inputJSON}, nil
	}
	inputForEvent := map[string]string{"goal": goal, "taskId": taskID}
	if metadataBool(task.Metadata, "batchMode") {
		inputForEvent["batchMode"] = "true"
		inputForEvent["batchParentTaskId"] = metadataString(task.Metadata, "batchParentTaskId")
		inputForEvent["batchWaveId"] = metadataString(task.Metadata, "batchWaveId")
	}
	inputJSON, _ := json.Marshal(inputForEvent)
	return types.HostOpRequest{
		Op:    types.HostOpToolResult,
		Tag:   "task_create",
		Text:  msg,
		Input: inputJSON,
	}, nil
}

var taskCreateCanonicalKeys = []string{
	"assignedRole",
	"goal",
	"inputs",
	"metadata",
	"priority",
	"spawnWorker",
	"taskId",
}

var taskCreateAliasToCanonical = map[string]string{
	"assigned_role": "assignedRole",
	"spawn_worker":  "spawnWorker",
	"task_id":       "taskId",
}

func normalizeTaskCreateArgs(args json.RawMessage) (json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return args, nil
	}

	allowed := map[string]struct{}{}
	for _, key := range taskCreateCanonicalKeys {
		allowed[key] = struct{}{}
	}
	for alias := range taskCreateAliasToCanonical {
		allowed[alias] = struct{}{}
	}

	unknown := make([]string, 0)
	for key := range raw {
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf(
			"task_create: unknown field(s): %s; supported fields: %s",
			strings.Join(unknown, ", "),
			strings.Join(taskCreateCanonicalKeys, ", "),
		)
	}

	out := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		canonical := key
		if mapped, ok := taskCreateAliasToCanonical[key]; ok {
			canonical = mapped
		}
		if existing, ok := out[canonical]; ok && !rawJSONEqual(existing, value) {
			return nil, fmt.Errorf(
				"task_create: conflicting values for %q via aliases; provide exactly one of %q or %q",
				canonical,
				canonical,
				aliasForCanonical(canonical),
			)
		}
		out[canonical] = value
	}

	normalized, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func aliasForCanonical(canonical string) string {
	for alias, mapped := range taskCreateAliasToCanonical {
		if mapped == canonical {
			return alias
		}
	}
	return canonical
}

func rawJSONEqual(a, b json.RawMessage) bool {
	var left any
	var right any
	if err := json.Unmarshal(a, &left); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
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

func metadataString(m map[string]any, key string) string {
	if len(m) == 0 {
		return ""
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func metadataBool(m map[string]any, key string) bool {
	if len(m) == 0 {
		return false
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}
