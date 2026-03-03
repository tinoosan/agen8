package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/types"
)

type failNthUpdateTaskStore struct {
	state.TaskStore
	failAt int
	calls  int
	err    error
}

func (s *failNthUpdateTaskStore) UpdateTask(ctx context.Context, task types.Task) error {
	s.calls++
	if s.calls == s.failAt {
		return s.err
	}
	return s.TaskStore.UpdateTask(ctx, task)
}

func TestRunTask_ReturnsErrorWhenMarkActiveFails(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	baseStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	updateErr := errors.New("update failed")
	wrapped := &failNthUpdateTaskStore{
		TaskStore: baseStore,
		failAt:    1,
		err:       updateErr,
	}

	sess, err := New(Config{
		Agent:        newFakeAgent(),
		Profile:      &profile.Profile{ID: "general"},
		TaskStore:    wrapped,
		SessionID:    "sess-1",
		RunID:        "run-1",
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	task := types.Task{
		TaskID:         "task-1",
		SessionID:      "sess-1",
		RunID:          "run-1",
		AssignedToType: "agent",
		AssignedTo:     "run-1",
		Goal:           "do work",
		Status:         types.TaskStatusPending,
		CreatedAt:      ptrTime(time.Now().UTC()),
	}
	if err := baseStore.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err = sess.runTask(context.Background(), task.TaskID, task)
	if err == nil {
		t.Fatal("expected update error")
	}
	if !errors.Is(err, updateErr) {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestRunTask_ReturnsErrorWhenFinalPersistenceUpdateFails(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	baseStore, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	updateErr := errors.New("final update failed")
	wrapped := &failNthUpdateTaskStore{
		TaskStore: baseStore,
		failAt:    2,
		err:       updateErr,
	}

	sess, err := New(Config{
		Agent:        newFakeAgent(),
		Profile:      &profile.Profile{ID: "general"},
		TaskStore:    wrapped,
		SessionID:    "sess-1",
		RunID:        "run-1",
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	task := types.Task{
		TaskID:         "task-1",
		SessionID:      "sess-1",
		RunID:          "run-1",
		AssignedToType: "agent",
		AssignedTo:     "run-1",
		Goal:           "do work",
		Status:         types.TaskStatusPending,
		CreatedAt:      ptrTime(time.Now().UTC()),
	}
	if err := baseStore.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err = sess.runTask(context.Background(), task.TaskID, task)
	if err == nil {
		t.Fatal("expected update error")
	}
	if !errors.Is(err, updateErr) {
		t.Fatalf("expected final update error, got %v", err)
	}

	loaded, err := baseStore.GetTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.Status != types.TaskStatusSucceeded {
		t.Fatalf("status=%q want %q", loaded.Status, types.TaskStatusSucceeded)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
