package hosttools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
	_ "modernc.org/sqlite"
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

func TestTaskReview_Execute_BatchItemApprove(t *testing.T) {
	cfg := setupTaskReviewStore(t)
	tool := &TaskReviewTool{Store: cfg.store, SessionID: "sess", RunID: "parent"}

	args, _ := json.Marshal(map[string]string{
		"taskId":          "callback-batch-parent-1",
		"batchItemTaskId": "callback-task-1",
		"decision":        "approve",
	})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpToolResult || req.Tag != "task_review" {
		t.Fatalf("unexpected response: %+v", req)
	}
	updatedBatch, err := cfg.store.GetTask(context.Background(), "callback-batch-parent-1")
	if err != nil {
		t.Fatalf("GetTask batch: %v", err)
	}
	decisions, _ := updatedBatch.Metadata["batchItemDecisions"].(map[string]any)
	if strings.TrimSpace(fmt.Sprint(decisions["callback-task-1"])) != "approve" {
		t.Fatalf("expected batch decision to be recorded, got %v", decisions)
	}
	updatedChild, err := cfg.store.GetTask(context.Background(), "callback-task-1")
	if err != nil {
		t.Fatalf("GetTask child: %v", err)
	}
	if updatedChild.Status != types.TaskStatusSucceeded {
		t.Fatalf("child status=%q want succeeded", updatedChild.Status)
	}
	if got := strings.TrimSpace(fmt.Sprint(updatedChild.Metadata["batchItemStatus"])); got != "approved" {
		t.Fatalf("batchItemStatus=%q want approved", got)
	}
	handoff, err := cfg.store.GetTask(context.Background(), "review-handoff-callback-batch-parent-1")
	if err != nil {
		all, _ := cfg.store.ListTasks(context.Background(), state.TaskFilter{Limit: 100, SortBy: "created_at"})
		ids := make([]string, 0, len(all))
		for _, tk := range all {
			ids = append(ids, tk.TaskID+":"+string(tk.Status))
		}
		t.Fatalf("expected handoff task: %v (tasks=%v)", err, ids)
	}
	if got := strings.TrimSpace(fmt.Sprint(handoff.Metadata["source"])); got != "review.handoff" {
		t.Fatalf("handoff source=%q", got)
	}
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("idempotent batch close should not fail: %v", err)
	}
	handoffAgain, err := cfg.store.GetTask(context.Background(), "review-handoff-callback-batch-parent-1")
	if err != nil {
		t.Fatalf("expected handoff task on re-run: %v", err)
	}
	if handoffAgain.TaskID != handoff.TaskID {
		t.Fatalf("expected deterministic handoff id, got %q vs %q", handoffAgain.TaskID, handoff.TaskID)
	}
}

func TestTaskReview_Execute_TeamBatchHandoffRoutesToCoordinatorRunAndCarriesArtifacts(t *testing.T) {
	cfg := setupTaskReviewStore(t)
	tool := &TaskReviewTool{Store: cfg.store, SessionID: "sess", RunID: "run-reviewer"}

	db, err := sql.Open("sqlite", cfg.dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			status TEXT NOT NULL,
			goal TEXT NOT NULL,
			run_json TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			created_at TEXT,
			updated_at TEXT,
			parent_run_id TEXT DEFAULT ''
		)`); err != nil {
		t.Fatalf("create runs table: %v", err)
	}
	nowRaw := time.Now().UTC().Format(time.RFC3339Nano)
	runJSON := `{"runId":"run-coord","sessionId":"sess-main","goal":"coord","status":"running","runtime":{"teamId":"team-1","role":"coordinator"}}`
	if _, err := db.Exec(`
		INSERT OR REPLACE INTO runs (run_id, session_id, status, goal, run_json, created_at, updated_at, parent_run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, '')
	`, "run-coord", "sess-main", "running", "coord", runJSON, nowRaw, nowRaw); err != nil {
		t.Fatalf("insert coordinator run: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	child := types.Task{
		TaskID:         "callback-team-child-1",
		SessionID:      "team-team-1",
		RunID:          "team-team-1-callback",
		TeamID:         "team-1",
		AssignedRole:   "reviewer",
		AssignedToType: "role",
		AssignedTo:     "reviewer",
		Goal:           "Review child",
		Status:         types.TaskStatusReviewPending,
		CreatedAt:      &now,
		Inputs: map[string]any{
			"artifacts": []string{"/workspace/reviewer/reports/review.md"},
		},
		Metadata: map[string]any{
			"source":            "team.callback",
			"callbackForTaskId": "task-team-child-1",
			"sourceRole":        "designer",
		},
	}
	if err := cfg.store.CreateTask(ctx, child); err != nil {
		t.Fatalf("create child callback: %v", err)
	}
	batch := types.Task{
		TaskID:         "callback-batch-team-parent-1",
		SessionID:      "team-team-1",
		RunID:          "team-team-1-callback",
		TeamID:         "team-1",
		AssignedRole:   "reviewer",
		AssignedToType: "role",
		AssignedTo:     "reviewer",
		Goal:           "Batch review",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Inputs: map[string]any{
			"items": []any{
				map[string]any{
					"callbackTaskId": "callback-team-child-1",
					"artifacts":      []any{"/workspace/reviewer/reports/review.md"},
					"decision":       "",
				},
			},
		},
		Metadata: map[string]any{
			"source":             "team.batch.callback",
			"coordinatorRole":    "coordinator",
			"batchParentTaskId":  "task-parent-team-1",
			"batchWaveId":        "wave-a",
			"batchItemDecisions": map[string]any{},
		},
	}
	if err := cfg.store.CreateTask(ctx, batch); err != nil {
		t.Fatalf("create team batch: %v", err)
	}

	args, _ := json.Marshal(map[string]string{
		"taskId":          "callback-batch-team-parent-1",
		"batchItemTaskId": "callback-team-child-1",
		"decision":        "approve",
	})
	if _, err := tool.Execute(ctx, args); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	handoff, err := cfg.store.GetTask(ctx, "review-handoff-callback-batch-team-parent-1")
	if err != nil {
		t.Fatalf("expected handoff task: %v", err)
	}
	if got := strings.TrimSpace(handoff.AssignedRole); got != "coordinator" {
		t.Fatalf("handoff assigned role=%q want coordinator", got)
	}
	if got := strings.TrimSpace(handoff.RunID); got != "run-coord" {
		t.Fatalf("handoff run=%q want run-coord", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(handoff.Metadata["reviewSummaryPath"])); got != "/workspace/reviewer/reports/review.md" {
		t.Fatalf("reviewSummaryPath=%q want reviewer report path", got)
	}
}

type taskReviewTestCfg struct {
	store  state.TaskStore
	dbPath string
}

type batchCloseTestStore struct {
	*state.SQLiteTaskStore
}

func (s *batchCloseTestStore) CloseBatchAndHandoff(ctx context.Context, batchTaskID, reviewerIdentity, reviewSummary string) (string, error) {
	handoffTaskID, _, _, _, err := s.SQLiteTaskStore.CloseBatchAndHandoffAtomic(ctx, batchTaskID, reviewerIdentity, reviewSummary)
	return handoffTaskID, err
}

func setupTaskReviewStore(t *testing.T) taskReviewTestCfg {
	t.Helper()
	dir := t.TempDir()
	path := fsutil.GetSQLitePath(dir)
	store, err := state.NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	closableStore := &batchCloseTestStore{SQLiteTaskStore: store}
	ctx := context.Background()
	now := time.Now().UTC()

	// Delegated task (spawn_worker) - agent might pass this ID.
	delegated := types.Task{
		TaskID: "task-1", SessionID: "sess", RunID: "parent",
		Goal: "sub work", Status: types.TaskStatusSucceeded,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "spawn_worker"},
	}
	if err := closableStore.CreateTask(ctx, delegated); err != nil {
		t.Fatalf("CreateTask delegated: %v", err)
	}

	// Callback task - the one that should be used for review.
	callback := types.Task{
		TaskID: "callback-task-1", SessionID: "sess", RunID: "parent",
		Goal: "Review result", Status: types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "subagent.callback", "sourceRunId": "child-run", "callbackForTaskId": "task-1"},
	}
	if err := closableStore.CreateTask(ctx, callback); err != nil {
		t.Fatalf("CreateTask callback: %v", err)
	}
	batch := types.Task{
		TaskID: "callback-batch-parent-1", SessionID: "sess", RunID: "parent",
		Goal: "Batch callback", Status: types.TaskStatusPending,
		CreatedAt: &now,
		Inputs: map[string]any{
			"items": []any{
				map[string]any{
					"callbackTaskId": "callback-task-1",
					"decision":       "",
				},
			},
		},
		Metadata: map[string]any{
			"source":             "subagent.batch.callback",
			"batchParentTaskId":  "task-parent-1",
			"batchItemDecisions": map[string]any{},
		},
	}
	if err := closableStore.CreateTask(ctx, batch); err != nil {
		t.Fatalf("CreateTask batch: %v", err)
	}

	// Another task that is not reviewable.
	other := types.Task{
		TaskID: "other-task", SessionID: "sess", RunID: "parent",
		Goal: "other", Status: types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": "user"},
	}
	if err := closableStore.CreateTask(ctx, other); err != nil {
		t.Fatalf("CreateTask other: %v", err)
	}

	return taskReviewTestCfg{store: closableStore, dbPath: path}
}
