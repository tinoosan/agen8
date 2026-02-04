package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestSQLiteStore_ClaimAndComplete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	ctx := context.Background()
	task := types.Task{
		TaskID:      "t1",
		SessionID:   "sess-1",
		RunID:       "run-1",
		Goal:        "test",
		Priority:    0,
		Status:      types.TaskStatusPending,
		CreatedAt:   ptrTime(time.Now()),
		Metadata:    map[string]any{"source": "test"},
		Inputs:      map[string]any{},
		TotalTokens: 0,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := s.ClaimTask(ctx, "t1", 200*time.Millisecond); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	claimed, err := s.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if claimed.Status != types.TaskStatusActive || claimed.Attempts != 1 {
		t.Fatalf("unexpected task after claim: %+v", claimed)
	}

	err = s.ClaimTask(ctx, "t1", 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected second claim to fail while leased")
	}
	if !errors.Is(err, ErrTaskClaimed) {
		t.Fatalf("expected ErrTaskClaimed, got: %v", err)
	}

	doneAt := time.Now()
	if err := s.CompleteTask(ctx, "t1", types.TaskResult{TaskID: "t1", Status: types.TaskStatusSucceeded, Summary: "ok", CompletedAt: &doneAt}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	rec, err := s.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Status != types.TaskStatusSucceeded {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestSQLiteStore_Claim_Expires(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	task := types.Task{TaskID: "t1", SessionID: "sess-1", RunID: "run-1", Goal: "test", Status: types.TaskStatusPending, CreatedAt: ptrTime(time.Now())}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := s.ClaimTask(ctx, "t1", 10*time.Millisecond); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := s.ClaimTask(ctx, "t1", 50*time.Millisecond); err != nil {
		t.Fatalf("ClaimTask(2): %v", err)
	}
	got2, err := s.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got2.Attempts < 2 {
		t.Fatalf("expected re-claim after lease expiry, got attempts=%d", got2.Attempts)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
