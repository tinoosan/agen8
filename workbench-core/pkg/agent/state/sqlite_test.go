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

func TestSQLiteStore_ListTasks_FilterByTeamAndRole(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	tasks := []types.Task{
		{
			TaskID:       "team-task-1",
			SessionID:    "sess-1",
			RunID:        "run-1",
			TeamID:       "team-1",
			AssignedRole: "researcher",
			CreatedBy:    "head-analyst",
			Goal:         "collect data",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata:     map[string]any{},
			Inputs:       map[string]any{},
		},
		{
			TaskID:       "team-task-2",
			SessionID:    "sess-2",
			RunID:        "run-2",
			TeamID:       "team-1",
			AssignedRole: "report-writer",
			CreatedBy:    "head-analyst",
			Goal:         "write brief",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata:     map[string]any{},
			Inputs:       map[string]any{},
		},
		{
			TaskID:       "team-task-3",
			SessionID:    "sess-3",
			RunID:        "run-3",
			TeamID:       "team-2",
			AssignedRole: "researcher",
			CreatedBy:    "lead",
			Goal:         "other team work",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata:     map[string]any{},
			Inputs:       map[string]any{},
		},
		{
			TaskID:       "team-task-4",
			SessionID:    "sess-4",
			RunID:        "run-4",
			TeamID:       "team-1",
			AssignedRole: "",
			CreatedBy:    "head-analyst",
			Goal:         "unassigned triage",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata:     map[string]any{},
			Inputs:       map[string]any{},
		},
	}
	for _, task := range tasks {
		task := task
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}

	got, err := s.ListTasks(ctx, TaskFilter{
		TeamID:       "team-1",
		AssignedRole: "researcher",
		Status:       []types.TaskStatus{types.TaskStatusPending},
		SortBy:       "created_at",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].TaskID != "team-task-1" {
		t.Fatalf("unexpected task ID: %s", got[0].TaskID)
	}
	if got[0].CreatedBy != "head-analyst" {
		t.Fatalf("unexpected createdBy: %s", got[0].CreatedBy)
	}

	unassigned, err := s.ListTasks(ctx, TaskFilter{
		TeamID:         "team-1",
		UnassignedOnly: true,
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListTasks(unassigned): %v", err)
	}
	if len(unassigned) != 1 {
		t.Fatalf("expected 1 unassigned task, got %d", len(unassigned))
	}
	if unassigned[0].TaskID != "team-task-4" {
		t.Fatalf("unexpected unassigned task: %s", unassigned[0].TaskID)
	}
}

func TestSQLiteStore_CancelActiveTasksByRun(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	tasks := []types.Task{
		{TaskID: "active-a", SessionID: "sess-1", RunID: "run-a", Goal: "a", Status: types.TaskStatusActive, CreatedAt: &now},
		{TaskID: "active-b", SessionID: "sess-1", RunID: "run-a", Goal: "b", Status: types.TaskStatusActive, CreatedAt: &now},
		{TaskID: "pending-c", SessionID: "sess-1", RunID: "run-a", Goal: "c", Status: types.TaskStatusPending, CreatedAt: &now},
		{TaskID: "active-d", SessionID: "sess-1", RunID: "run-b", Goal: "d", Status: types.TaskStatusActive, CreatedAt: &now},
	}
	for _, task := range tasks {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}

	count, err := s.CancelActiveTasksByRun(ctx, "run-a", "run paused")
	if err != nil {
		t.Fatalf("CancelActiveTasksByRun: %v", err)
	}
	if count != 2 {
		t.Fatalf("canceled count=%d want 2", count)
	}

	activeA, err := s.GetTask(ctx, "active-a")
	if err != nil {
		t.Fatalf("GetTask(active-a): %v", err)
	}
	if activeA.Status != types.TaskStatusCanceled {
		t.Fatalf("active-a status=%q want %q", activeA.Status, types.TaskStatusCanceled)
	}
	pendingC, err := s.GetTask(ctx, "pending-c")
	if err != nil {
		t.Fatalf("GetTask(pending-c): %v", err)
	}
	if pendingC.Status != types.TaskStatusPending {
		t.Fatalf("pending-c status=%q want %q", pendingC.Status, types.TaskStatusPending)
	}
	activeD, err := s.GetTask(ctx, "active-d")
	if err != nil {
		t.Fatalf("GetTask(active-d): %v", err)
	}
	if activeD.Status != types.TaskStatusActive {
		t.Fatalf("active-d status=%q want %q", activeD.Status, types.TaskStatusActive)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
