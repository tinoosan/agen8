package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Ensure Manager implements interfaces at compile time.
var (
	_ state.TaskStore            = (*Manager)(nil)
	_ RetryEscalationCreator     = (*Manager)(nil)
	_ ActiveTaskCanceler         = (*Manager)(nil)
	_ ArtifactIndexerProvider    = (*Manager)(nil)
	_ TaskServiceForRPC          = (*Manager)(nil)
	_ TaskServiceForSupervisor   = (*Manager)(nil)
	_ TaskServiceForTeam         = (*Manager)(nil)
)

type mockRunLoader struct {
	run types.Run
	err error
}

func (m *mockRunLoader) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	if m.err != nil {
		return types.Run{}, m.err
	}
	return m.run, nil
}

type mockTaskStore struct {
	getTask    func(ctx context.Context, taskID string) (types.Task, error)
	listTasks  func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error)
	createTask func(ctx context.Context, task types.Task) error
	completeTask func(ctx context.Context, taskID string, result types.TaskResult) error
}

func (m *mockTaskStore) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	if m.getTask != nil {
		return m.getTask(ctx, taskID)
	}
	return types.Task{}, state.ErrTaskNotFound
}

func (m *mockTaskStore) GetRunStats(ctx context.Context, runID string) (state.RunStats, error) {
	return state.RunStats{}, nil
}

func (m *mockTaskStore) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	if m.listTasks != nil {
		return m.listTasks(ctx, filter)
	}
	return nil, nil
}

func (m *mockTaskStore) CountTasks(ctx context.Context, filter state.TaskFilter) (int, error) {
	return 0, nil
}

func (m *mockTaskStore) CreateTask(ctx context.Context, task types.Task) error {
	if m.createTask != nil {
		return m.createTask(ctx, task)
	}
	return nil
}

func (m *mockTaskStore) DeleteTask(ctx context.Context, taskID string) error { return nil }
func (m *mockTaskStore) UpdateTask(ctx context.Context, task types.Task) error { return nil }

func (m *mockTaskStore) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	if m.completeTask != nil {
		return m.completeTask(ctx, taskID, result)
	}
	return nil
}

func (m *mockTaskStore) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	return nil
}
func (m *mockTaskStore) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	return nil
}
func (m *mockTaskStore) ReleaseLease(ctx context.Context, taskID string) error { return nil }
func (m *mockTaskStore) DelegateTask(ctx context.Context, taskID string) error { return nil }
func (m *mockTaskStore) ResumeTask(ctx context.Context, taskID string) error   { return nil }
func (m *mockTaskStore) RecoverExpiredLeases(ctx context.Context) error       { return nil }

func TestManager_CreateRetryTask_NoRunLoader(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, nil)
	err := mgr.CreateRetryTask(context.Background(), "run-1", "feedback")
	if err == nil {
		t.Fatal("expected error when run loader is nil")
	}
	if !strings.Contains(err.Error(), "run loader not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_CreateRetryTask_EmptyChildRunID(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, &mockRunLoader{})
	err := mgr.CreateRetryTask(context.Background(), "", "feedback")
	if err == nil {
		t.Fatal("expected error for empty childRunID")
	}
}

func TestManager_CreateRetryTask_LoadRunError(t *testing.T) {
	loader := &mockRunLoader{err: errors.New("load failed")}
	mgr := NewManager(&mockTaskStore{}, loader)
	err := mgr.CreateRetryTask(context.Background(), "run-1", "feedback")
	if err == nil {
		t.Fatal("expected error when LoadRun fails")
	}
	if !errors.Is(err, loader.err) && !strings.Contains(err.Error(), "load failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_CreateRetryTask_Success(t *testing.T) {
	run := types.Run{
		RunID:       "run-child",
		SessionID:   "sess-1",
		Goal:        "original goal",
		ParentRunID: "run-parent",
	}
	loader := &mockRunLoader{run: run}
	var created types.Task
	store := &mockTaskStore{
		createTask: func(ctx context.Context, task types.Task) error {
			created = task
			return nil
		},
	}
	mgr := NewManager(store, loader)
	err := mgr.CreateRetryTask(context.Background(), "run-child", "do it again")
	if err != nil {
		t.Fatalf("CreateRetryTask: %v", err)
	}
	if created.SessionID != "sess-1" || created.RunID != "run-child" {
		t.Errorf("created task session/run: %q / %q", created.SessionID, created.RunID)
	}
	if !strings.Contains(created.Goal, "do it again") || !strings.Contains(created.Goal, "original goal") {
		t.Errorf("goal: %s", created.Goal)
	}
	if created.Metadata["source"] != "retry" || created.Metadata["parentRunId"] != "run-parent" {
		t.Errorf("metadata: %+v", created.Metadata)
	}
}

func TestManager_CancelActiveTasksByRun_Fallback(t *testing.T) {
	var completed []string
	store := &mockTaskStore{
		listTasks: func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
			return []types.Task{
				{TaskID: "t1", RunID: "run-1", Status: types.TaskStatusActive},
				{TaskID: "t2", RunID: "run-1", Status: types.TaskStatusActive},
			}, nil
		},
		completeTask: func(ctx context.Context, taskID string, result types.TaskResult) error {
			completed = append(completed, taskID)
			return nil
		},
	}
	mgr := NewManager(store, nil)
	n, err := mgr.CancelActiveTasksByRun(context.Background(), "run-1", "paused")
	if err != nil {
		t.Fatalf("CancelActiveTasksByRun: %v", err)
	}
	if n != 2 {
		t.Errorf("count: got %d want 2", n)
	}
	if len(completed) != 2 || (completed[0] != "t1" && completed[0] != "t2") {
		t.Errorf("completed: %v", completed)
	}
}

func TestManager_ArtifactIndexer_NotImplemented(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, nil)
	idx, ok := mgr.ArtifactIndexer()
	if ok || idx != nil {
		t.Errorf("ArtifactIndexer: want (nil, false), got (%v, %v)", idx, ok)
	}
}
