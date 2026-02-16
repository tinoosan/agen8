package hosttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestTaskReview_Execute_AcceptsCallbackTaskID(t *testing.T) {
	cfg := setupTaskReviewStore(t)
	tool := &TaskReviewTool{Store: cfg.store, SessionID: "sess", RunID: "parent"}

	args, _ := json.Marshal(map[string]string{
		"taskId":   "callback-task-1",
		"decision": "approve",
	})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpToolResult || req.Tag != "task_review" {
		t.Errorf("expected tool_result task_review, got op=%q tag=%q", req.Op, req.Tag)
	}
}

func TestTaskReview_Execute_AcceptsDelegatedTaskID_ResolvesToCallback(t *testing.T) {
	cfg := setupTaskReviewStore(t)
	tool := &TaskReviewTool{Store: cfg.store, SessionID: "sess", RunID: "parent"}

	// Pass the delegated task ID (source=spawn_worker); tool should resolve to callback and approve.
	args, _ := json.Marshal(map[string]string{
		"taskId":   "task-1",
		"decision": "approve",
	})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpToolResult || req.Tag != "task_review" {
		t.Errorf("expected tool_result task_review, got op=%q tag=%q", req.Op, req.Tag)
	}
}

func TestTaskReview_Execute_RejectsNonReviewableTask(t *testing.T) {
	cfg := setupTaskReviewStore(t)
	tool := &TaskReviewTool{Store: cfg.store, SessionID: "sess", RunID: "parent"}

	args, _ := json.Marshal(map[string]string{
		"taskId":   "other-task",
		"decision": "approve",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for non-reviewable task")
	}
	if err != nil && !strings.Contains(err.Error(), "not a reviewable callback") {
		t.Errorf("error should mention reviewable callback: %v", err)
	}
}

type taskReviewTestCfg struct {
	store state.TaskStore
}

func setupTaskReviewStore(t *testing.T) taskReviewTestCfg {
	t.Helper()
	dir := t.TempDir()
	path := fsutil.GetSQLitePath(dir)
	store, err := state.NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	// Delegated task (spawn_worker) - agent might pass this ID.
	delegated := types.Task{
		TaskID: "task-1", SessionID: "sess", RunID: "parent",
		Goal: "sub work", Status: types.TaskStatusSucceeded,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "spawn_worker"},
	}
	if err := store.CreateTask(ctx, delegated); err != nil {
		t.Fatalf("CreateTask delegated: %v", err)
	}

	// Callback task - the one that should be used for review.
	callback := types.Task{
		TaskID: "callback-task-1", SessionID: "sess", RunID: "parent",
		Goal: "Review result", Status: types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "subagent.callback", "sourceRunId": "child-run", "callbackForTaskId": "task-1"},
	}
	if err := store.CreateTask(ctx, callback); err != nil {
		t.Fatalf("CreateTask callback: %v", err)
	}

	// Another task that is not reviewable.
	other := types.Task{
		TaskID: "other-task", SessionID: "sess", RunID: "parent",
		Goal: "other", Status: types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "user"},
	}
	if err := store.CreateTask(ctx, other); err != nil {
		t.Fatalf("CreateTask other: %v", err)
	}

	return taskReviewTestCfg{store: store}
}
