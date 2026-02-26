package session

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/harness"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/types"
)

type testHarnessAdapter struct {
	id     string
	result harness.TaskResult
	err    error

	mu     sync.Mutex
	calls  int
	lastRq harness.TaskRequest
}

func (a *testHarnessAdapter) ID() string { return a.id }

func (a *testHarnessAdapter) RunTask(_ context.Context, req harness.TaskRequest) (harness.TaskResult, error) {
	a.mu.Lock()
	a.calls++
	a.lastRq = req
	a.mu.Unlock()
	if a.err != nil {
		return harness.TaskResult{}, a.err
	}
	return a.result, nil
}

func (a *testHarnessAdapter) called() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func TestSessionRunTask_UsesExternalHarnessAdapter(t *testing.T) {
	ctx := context.Background()
	taskStore, _ := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	now := time.Now().UTC()

	adapter := &testHarnessAdapter{
		id: "fake-external",
		result: harness.TaskResult{
			Status:       types.TaskStatusSucceeded,
			Text:         "external-result",
			InputTokens:  11,
			OutputTokens: 7,
			TotalTokens:  18,
			CostUSD:      0.0123,
			AdapterRunID: "external-run-1",
		},
	}
	reg, err := harness.NewRegistry(adapter)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	task := types.Task{
		TaskID:         "task-1",
		SessionID:      "sess-1",
		RunID:          "run-1",
		AssignedToType: "agent",
		AssignedTo:     "run-1",
		TaskKind:       state.TaskKindTask,
		Goal:           "execute with external harness",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Metadata:       map[string]any{"harnessId": "fake-external"},
	}
	if err := taskStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	ag := newFakeAgent()
	events := &captureEventEmitter{}
	s, err := New(Config{
		Agent:           ag,
		Profile:         &profile.Profile{ID: "general"},
		TaskStore:       taskStore,
		Events:          events,
		SessionID:       "sess-1",
		RunID:           "run-1",
		Workdir:         t.TempDir(),
		HarnessRegistry: reg,
		PollInterval:    20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := s.drainInbox(ctx); err != nil {
		t.Fatalf("drainInbox: %v", err)
	}

	stored, err := taskStore.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != types.TaskStatusSucceeded {
		t.Fatalf("task status = %q want %q", stored.Status, types.TaskStatusSucceeded)
	}
	if stored.Summary == "" || stored.TotalTokens != 18 {
		t.Fatalf("unexpected task summary/tokens: summary=%q tokens=%d", stored.Summary, stored.TotalTokens)
	}
	if got := stored.CostUSD; got != 0 {
		t.Fatalf("expected external harness cost to remain zero, got %.4f", got)
	}
	if got := stored.Metadata["harnessId"]; got != "fake-external" {
		t.Fatalf("metadata harnessId = %#v", got)
	}
	if got := stored.Metadata["harnessRunRef"]; got != "external-run-1" {
		t.Fatalf("metadata harnessRunRef = %#v", got)
	}
	if calls := adapter.called(); calls != 1 {
		t.Fatalf("adapter calls = %d want 1", calls)
	}
	if runs := ag.runs(); runs != 0 {
		t.Fatalf("native agent should not run for external harness, got %d runs", runs)
	}
	if ev, ok := events.firstByType("harness.run.complete"); !ok {
		t.Fatalf("expected harness.run.complete event")
	} else if ev.Data["harnessId"] != "fake-external" {
		t.Fatalf("harness.run.complete harnessId = %q", ev.Data["harnessId"])
	}
}

func TestSessionRunTask_ExternalHarnessFailureMarksTaskFailed(t *testing.T) {
	ctx := context.Background()
	taskStore, _ := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	now := time.Now().UTC()

	adapter := &testHarnessAdapter{
		id: "fake-fail",
		result: harness.TaskResult{
			Status: types.TaskStatusFailed,
			Error:  "adapter failed",
		},
	}
	reg, err := harness.NewRegistry(adapter)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if err := taskStore.CreateTask(ctx, types.Task{
		TaskID:         "task-1",
		SessionID:      "sess-1",
		RunID:          "run-1",
		AssignedToType: "agent",
		AssignedTo:     "run-1",
		TaskKind:       state.TaskKindTask,
		Goal:           "should fail",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Metadata:       map[string]any{"harnessId": "fake-fail"},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	s, err := New(Config{
		Agent:           newFakeAgent(),
		Profile:         &profile.Profile{ID: "general"},
		TaskStore:       taskStore,
		SessionID:       "sess-1",
		RunID:           "run-1",
		HarnessRegistry: reg,
		PollInterval:    20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := s.drainInbox(ctx); err != nil {
		t.Fatalf("drainInbox: %v", err)
	}

	stored, err := taskStore.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != types.TaskStatusFailed {
		t.Fatalf("task status = %q want %q", stored.Status, types.TaskStatusFailed)
	}
	if stored.Error == "" {
		t.Fatalf("expected task error to be populated")
	}
}
