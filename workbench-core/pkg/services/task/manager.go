package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// activeTaskCanceler is implemented by stores that support efficient cancel-by-run (e.g. SQLite).
type activeTaskCanceler interface {
	CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error)
}

// Manager implements state.TaskStore, RetryEscalationCreator, ActiveTaskCanceler, and ArtifactIndexerProvider.
// It delegates CRUD to the store and implements callback lifecycle (retry/escalation) and cancel-by-run.
type Manager struct {
	store     state.TaskStore
	runLoader RunLoader
}

// NewManager creates a new task service manager. runLoader may be nil and set later via SetRunLoader.
func NewManager(store state.TaskStore, runLoader RunLoader) *Manager {
	return &Manager{
		store:     store,
		runLoader: runLoader,
	}
}

// SetRunLoader sets the run loader (e.g. after session service is constructed in daemon wiring).
func (m *Manager) SetRunLoader(runLoader RunLoader) {
	m.runLoader = runLoader
}

// CreateRetryTask creates a retry task for a child run (loads run via RunLoader, builds task, persists).
func (m *Manager) CreateRetryTask(ctx context.Context, childRunID, feedback string) error {
	childRunID = strings.TrimSpace(childRunID)
	if childRunID == "" {
		return fmt.Errorf("childRunID is required")
	}
	if m.runLoader == nil {
		return fmt.Errorf("run loader not configured")
	}
	run, err := m.runLoader.LoadRun(ctx, childRunID)
	if err != nil {
		return fmt.Errorf("load child run: %w", err)
	}
	retryTask := types.Task{
		TaskID:         fmt.Sprintf("retry-%s-%s", childRunID, uuid.NewString()[:8]),
		SessionID:      run.SessionID,
		RunID:          childRunID,
		AssignedToType: "agent",
		AssignedTo:     childRunID,
		TaskKind:       "task",
		Goal:           fmt.Sprintf("RETRY with feedback:\n%s\n\nOriginal goal: %s", feedback, strings.TrimSpace(run.Goal)),
		Priority:       1,
		Status:         types.TaskStatusPending,
		Metadata: map[string]any{
			"source":        "retry",
			"parentRunId":   strings.TrimSpace(run.ParentRunID),
			"retryFeedback": feedback,
		},
	}
	return m.store.CreateTask(ctx, retryTask)
}

// CreateEscalationTask creates an escalation task from a callback task (loads callback via store, builds escalation task, persists).
func (m *Manager) CreateEscalationTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error {
	callbackTaskID = strings.TrimSpace(callbackTaskID)
	if callbackTaskID == "" {
		return fmt.Errorf("callbackTaskID is required")
	}
	task, err := m.store.GetTask(ctx, callbackTaskID)
	if err != nil {
		return fmt.Errorf("load callback task: %w", err)
	}
	now := time.Now().UTC()
	escalationTaskID := fmt.Sprintf("escalation-%s-%s", callbackTaskID, uuid.NewString()[:8])
	escMeta := map[string]any{
		"source":         "escalation",
		"reason":         data.Reason,
		"attemptSummary": data.AttemptSummary,
		"recommendation": data.Recommendation,
		"originalGoal":   data.OriginalGoal,
		"retryCount":     data.RetryCount,
		"sourceRunId":    data.SourceRunID,
		"sourceTaskId":   data.SourceTaskID,
	}
	if len(data.Artifacts) > 0 {
		escMeta["artifacts"] = data.Artifacts
	}
	escalationGoal := fmt.Sprintf("ESCALATION: %s\n\nOriginal goal: %s\nAttempts: %d\nRecommendation: %s",
		data.Reason, data.OriginalGoal, data.RetryCount, data.Recommendation)
	escalationTask := types.Task{
		TaskID:    escalationTaskID,
		SessionID: task.SessionID,
		TaskKind:  "task",
		Goal:      escalationGoal,
		Priority:  0,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  escMeta,
	}
	teamID := strings.TrimSpace(task.TeamID)
	if teamID != "" {
		coordinatorRole := ""
		if cr, ok := task.Metadata["coordinatorRole"].(string); ok {
			coordinatorRole = strings.TrimSpace(cr)
		}
		if coordinatorRole == "" {
			coordinatorRole = strings.TrimSpace(task.AssignedRole)
		}
		escalationTask.TeamID = teamID
		escalationTask.AssignedRole = coordinatorRole
		escalationTask.AssignedToType = "role"
		escalationTask.AssignedTo = coordinatorRole
	} else {
		parentRunID := strings.TrimSpace(task.RunID)
		escalationTask.RunID = parentRunID
		escalationTask.AssignedToType = "agent"
		escalationTask.AssignedTo = parentRunID
	}
	if err := m.store.CreateTask(ctx, escalationTask); err != nil {
		return fmt.Errorf("create escalation task: %w", err)
	}
	return nil
}

// CancelActiveTasksByRun delegates to the store if it implements activeTaskCanceler, else falls back to ListTasks + CompleteTask.
func (m *Manager) CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "run paused"
	}
	if c, ok := m.store.(activeTaskCanceler); ok {
		return c.CancelActiveTasksByRun(ctx, runID, reason)
	}
	tasks, err := m.store.ListTasks(ctx, state.TaskFilter{
		RunID:  runID,
		Status: []types.TaskStatus{types.TaskStatusActive},
		Limit:  500,
	})
	if err != nil {
		return 0, err
	}
	doneAt := time.Now().UTC()
	for _, task := range tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}
		if err := m.store.CompleteTask(ctx, taskID, types.TaskResult{
			TaskID:      taskID,
			Status:      types.TaskStatusCanceled,
			CompletedAt: &doneAt,
			Error:       reason,
		}); err != nil {
			return len(tasks), err
		}
	}
	return len(tasks), nil
}

// ArtifactIndexer returns the underlying store as state.ArtifactIndexer if it implements it.
func (m *Manager) ArtifactIndexer() (state.ArtifactIndexer, bool) {
	if m.store == nil {
		return nil, false
	}
	idx, ok := m.store.(state.ArtifactIndexer)
	return idx, ok
}

// TaskStore delegation (state.TaskStore)

func (m *Manager) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	return m.store.GetTask(ctx, taskID)
}

func (m *Manager) GetRunStats(ctx context.Context, runID string) (state.RunStats, error) {
	return m.store.GetRunStats(ctx, runID)
}

func (m *Manager) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	return m.store.ListTasks(ctx, filter)
}

func (m *Manager) CountTasks(ctx context.Context, filter state.TaskFilter) (int, error) {
	return m.store.CountTasks(ctx, filter)
}

func (m *Manager) CreateTask(ctx context.Context, task types.Task) error {
	return m.store.CreateTask(ctx, task)
}

func (m *Manager) DeleteTask(ctx context.Context, taskID string) error {
	return m.store.DeleteTask(ctx, taskID)
}

func (m *Manager) UpdateTask(ctx context.Context, task types.Task) error {
	return m.store.UpdateTask(ctx, task)
}

func (m *Manager) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	return m.store.CompleteTask(ctx, taskID, result)
}

func (m *Manager) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	return m.store.ClaimTask(ctx, taskID, ttl)
}

func (m *Manager) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	return m.store.ExtendLease(ctx, taskID, ttl)
}

func (m *Manager) ReleaseLease(ctx context.Context, taskID string) error {
	return m.store.ReleaseLease(ctx, taskID)
}

func (m *Manager) DelegateTask(ctx context.Context, taskID string) error {
	return m.store.DelegateTask(ctx, taskID)
}

func (m *Manager) ResumeTask(ctx context.Context, taskID string) error {
	return m.store.ResumeTask(ctx, taskID)
}

func (m *Manager) RecoverExpiredLeases(ctx context.Context) error {
	return m.store.RecoverExpiredLeases(ctx)
}
