package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestCompleteTask_IndexesArtifactsWithTeamDiskPath(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTask(ctx, types.Task{
		TaskID:       "callback-task-ceo-1",
		SessionID:    "team-team-1",
		RunID:        "run-ceo-1",
		TeamID:       "team-1",
		AssignedRole: "ceo",
		CreatedBy:    "coordinator",
		Goal:         "review",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
		Inputs:       map[string]any{},
		Metadata:     map[string]any{},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(2 * time.Second)
	res := types.TaskResult{
		TaskID:      "callback-task-ceo-1",
		Status:      types.TaskStatusSucceeded,
		Summary:     "ok",
		CompletedAt: &done,
		Artifacts: []string{
			"/workspace/ceo/deliverables/2026-02-08/callback-task-ceo-1/SUMMARY.md",
			"/workspace/ceo/deliverables/2026-02-08/callback-task-ceo-1/report.md",
		},
	}
	if err := s.CompleteTask(ctx, "callback-task-ceo-1", res); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	rows, err := s.ListArtifactsByTask(ctx, ArtifactFilter{TaskID: "callback-task-ceo-1"})
	if err != nil {
		t.Fatalf("ListArtifactsByTask: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(rows))
	}
	if rows[0].TaskKind != TaskKindCallback {
		t.Fatalf("expected callback task kind, got %q", rows[0].TaskKind)
	}
	wantPrefix := filepath.Join(dir, "teams", "team-1", "workspace")
	if rows[0].DiskPath == "" || !strings.HasPrefix(rows[0].DiskPath, wantPrefix) {
		t.Fatalf("expected team disk path under %q, got %q", wantPrefix, rows[0].DiskPath)
	}
}

func TestListArtifactGroups_GroupedAndOrdered(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	tasks := []types.Task{
		{TaskID: "task-1", SessionID: "team-team-1", RunID: "run-1", TeamID: "team-1", AssignedRole: "ceo", CreatedBy: "coordinator", Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now, Inputs: map[string]any{}, Metadata: map[string]any{}},
		{TaskID: "heartbeat-ceo-run-1", SessionID: "team-team-1", RunID: "run-1", TeamID: "team-1", AssignedRole: "ceo", CreatedBy: "system", Goal: "HB", Status: types.TaskStatusPending, CreatedAt: &now, Inputs: map[string]any{}, Metadata: map[string]any{}},
	}
	for _, tk := range tasks {
		if err := s.CreateTask(ctx, tk); err != nil {
			t.Fatalf("CreateTask(%s): %v", tk.TaskID, err)
		}
	}
	done := now.Add(2 * time.Second)
	_ = s.CompleteTask(ctx, "task-1", types.TaskResult{TaskID: "task-1", Status: types.TaskStatusSucceeded, Summary: "ok", CompletedAt: &done, Artifacts: []string{"/workspace/ceo/deliverables/2026-02-08/task-1/SUMMARY.md"}})
	_ = s.CompleteTask(ctx, "heartbeat-ceo-run-1", types.TaskResult{TaskID: "heartbeat-ceo-run-1", Status: types.TaskStatusSucceeded, Summary: "ok", CompletedAt: &done, Artifacts: []string{"/workspace/ceo/deliverables/2026-02-08/heartbeat-ceo-run-1/SUMMARY.md"}})

	groups, err := s.ListArtifactGroups(ctx, ArtifactFilter{TeamID: "team-1"})
	if err != nil {
		t.Fatalf("ListArtifactGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 grouped tasks, got %d", len(groups))
	}
	if groups[0].DayBucket == "" || groups[0].Role == "" || groups[0].TaskID == "" {
		t.Fatalf("group missing required fields: %+v", groups[0])
	}
}

func TestSearchArtifacts_GlobalAndScoped(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.CreateTask(ctx, types.Task{TaskID: "task-1", SessionID: "team-team-1", RunID: "run-1", TeamID: "team-1", AssignedRole: "ceo", CreatedBy: "coordinator", Goal: "A", Status: types.TaskStatusPending, CreatedAt: &now, Inputs: map[string]any{}, Metadata: map[string]any{}}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(2 * time.Second)
	if err := s.CompleteTask(ctx, "task-1", types.TaskResult{TaskID: "task-1", Status: types.TaskStatusSucceeded, Summary: "ok", CompletedAt: &done, Artifacts: []string{"/workspace/ceo/deliverables/2026-02-08/task-1/findings.json"}}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	global, err := s.SearchArtifacts(ctx, ArtifactSearchFilter{ArtifactFilter: ArtifactFilter{TeamID: "team-1"}, Query: "findings"})
	if err != nil {
		t.Fatalf("SearchArtifacts(global): %v", err)
	}
	if len(global) != 1 {
		t.Fatalf("expected 1 global match, got %d", len(global))
	}
	scoped, err := s.SearchArtifacts(ctx, ArtifactSearchFilter{ArtifactFilter: ArtifactFilter{TeamID: "team-1", Role: "ceo"}, Query: "findings"})
	if err != nil {
		t.Fatalf("SearchArtifacts(scoped): %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("expected 1 scoped match, got %d", len(scoped))
	}
}

func TestBackfillArtifactIndex_ReconcilesMissingSummaryRows(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	taskID := "task-standalone-1"
	if err := s.CreateTask(ctx, types.Task{
		TaskID:    taskID,
		SessionID: "sess-1",
		RunID:     "run-1",
		Goal:      "reconcile artifacts",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Inputs:    map[string]any{},
		Metadata:  map[string]any{},
		CreatedBy: "user",
		TaskKind:  TaskKindTask,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(2 * time.Second)
	summaryVPath := "/tasks/2026-02-08/" + taskID + "/SUMMARY.md"
	reportVPath := "/tasks/2026-02-08/" + taskID + "/report.md"
	if err := s.CompleteTask(ctx, taskID, types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusSucceeded,
		Summary:     "ok",
		CompletedAt: &done,
		Artifacts:   []string{summaryVPath, reportVPath},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	db, err := s.dbConn()
	if err != nil {
		t.Fatalf("dbConn: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM artifacts WHERE task_id = ? AND vpath = ?`, taskID, summaryVPath); err != nil {
		t.Fatalf("delete summary row: %v", err)
	}

	before, err := s.ListArtifactsByTask(ctx, ArtifactFilter{TaskID: taskID})
	if err != nil {
		t.Fatalf("ListArtifactsByTask(before): %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected 1 artifact before reconcile, got %d", len(before))
	}

	if err := s.backfillArtifactIndex(ctx, db); err != nil {
		t.Fatalf("backfillArtifactIndex: %v", err)
	}

	after, err := s.ListArtifactsByTask(ctx, ArtifactFilter{TaskID: taskID})
	if err != nil {
		t.Fatalf("ListArtifactsByTask(after): %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 artifacts after reconcile, got %d", len(after))
	}
	foundSummary := false
	for _, row := range after {
		if strings.EqualFold(strings.TrimSpace(row.VPath), summaryVPath) && row.IsSummary {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected summary artifact row after reconcile, got %+v", after)
	}
}
