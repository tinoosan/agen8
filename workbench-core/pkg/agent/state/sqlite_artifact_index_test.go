package state

import (
	"context"
	"encoding/json"
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
			"/tasks/ceo/2026-02-08/callback-task-ceo-1/SUMMARY.md",
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
	if len(rows) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(rows))
	}
	if rows[0].TaskKind != TaskKindCallback {
		t.Fatalf("expected callback task kind, got %q", rows[0].TaskKind)
	}
	var foundTeamTasks bool
	var foundTeamWorkspace bool
	for _, row := range rows {
		if strings.HasPrefix(row.VPath, "/tasks/") {
			wantPrefix := filepath.Join(dir, "teams", "team-1", "tasks")
			if row.DiskPath == "" || !strings.HasPrefix(row.DiskPath, wantPrefix) {
				t.Fatalf("expected team task disk path under %q, got %q", wantPrefix, row.DiskPath)
			}
			foundTeamTasks = true
			continue
		}
		if strings.HasPrefix(row.VPath, "/workspace/") {
			wantPrefix := filepath.Join(dir, "teams", "team-1", "workspace")
			if row.DiskPath == "" || !strings.HasPrefix(row.DiskPath, wantPrefix) {
				t.Fatalf("expected team workspace disk path under %q, got %q", wantPrefix, row.DiskPath)
			}
			foundTeamWorkspace = true
		}
	}
	if !foundTeamTasks {
		t.Fatalf("expected a team /tasks artifact row, got %+v", rows)
	}
	if !foundTeamWorkspace {
		t.Fatalf("expected team /workspace artifact rows, got %+v", rows)
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

func TestCompleteTask_StandaloneSubagentArtifactsIndexedUnderParentCanonicalPaths(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	db, err := s.dbConn()
	if err != nil {
		t.Fatalf("dbConn: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runs (run_id TEXT PRIMARY KEY, run_json TEXT)`); err != nil {
		t.Fatalf("create runs table: %v", err)
	}
	childRun := types.Run{RunID: "run-child-1", ParentRunID: "run-parent-1", SpawnIndex: 3}
	rawRun, _ := json.Marshal(childRun)
	if _, err := db.ExecContext(ctx, `INSERT INTO runs (run_id, run_json) VALUES (?, ?)`, childRun.RunID, string(rawRun)); err != nil {
		t.Fatalf("insert child run: %v", err)
	}

	taskID := "task-child-1"
	if err := s.CreateTask(ctx, types.Task{
		TaskID:    taskID,
		SessionID: "sess-1",
		RunID:     childRun.RunID,
		Goal:      "child work",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Inputs:    map[string]any{},
		Metadata:  map[string]any{"parentRunId": "run-parent-1"},
		CreatedBy: "run-parent-1",
		TaskKind:  TaskKindTask,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(2 * time.Second)
	if err := s.CompleteTask(ctx, taskID, types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusSucceeded,
		CompletedAt: &done,
		Artifacts: []string{
			"/workspace/hello.txt",
			"/tasks/2026-02-17/task-child-1/SUMMARY.md",
		},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	rows, err := s.ListArtifactsByTask(ctx, ArtifactFilter{TaskID: taskID})
	if err != nil {
		t.Fatalf("ListArtifactsByTask: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(rows))
	}
	wantPaths := map[string]bool{
		"/workspace/subagent-3/hello.txt":                            false,
		"/tasks/subagent-3/2026-02-17/task-child-1/SUMMARY.md": false,
	}
	for _, row := range rows {
		if _, ok := wantPaths[row.VPath]; !ok {
			t.Fatalf("unexpected vpath %q", row.VPath)
		}
		wantPaths[row.VPath] = true
		if row.RunID != "run-parent-1" {
			t.Fatalf("expected parent run id, got %q", row.RunID)
		}
	}
	for vpath, seen := range wantPaths {
		if !seen {
			t.Fatalf("missing vpath %q in indexed artifacts", vpath)
		}
	}
}

func TestBackfillArtifactIndex_RewritesLegacyStandaloneSubagentSummaryPath(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteTaskStore(filepath.Join(dir, "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	db, err := s.dbConn()
	if err != nil {
		t.Fatalf("dbConn: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runs (run_id TEXT PRIMARY KEY, session_id TEXT, status TEXT, goal TEXT, run_json TEXT, created_at TEXT, updated_at TEXT, parent_run_id TEXT)`); err != nil {
		t.Fatalf("create runs table: %v", err)
	}
	childRun := types.Run{RunID: "run-child-9", ParentRunID: "run-parent-9", SpawnIndex: 1}
	rawRun, _ := json.Marshal(childRun)
	if _, err := db.ExecContext(ctx, `INSERT INTO runs (run_id, session_id, status, goal, run_json, created_at, updated_at, parent_run_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		childRun.RunID, "sess-1", "running", "child", string(rawRun), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), childRun.ParentRunID); err != nil {
		t.Fatalf("insert child run: %v", err)
	}

	taskID := "task-child-backfill-1"
	task := types.Task{
		TaskID:    taskID,
		SessionID: "sess-1",
		RunID:     childRun.RunID,
		Goal:      "child summary",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Inputs:    map[string]any{},
		Metadata:  map[string]any{"parentRunId": childRun.ParentRunID},
		CreatedBy: childRun.ParentRunID,
		TaskKind:  TaskKindTask,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	done := now.Add(2 * time.Second)
	legacyVPath := "/tasks/2026-02-17/" + taskID + "/SUMMARY.md"
	if err := s.CompleteTask(ctx, taskID, types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusSucceeded,
		CompletedAt: &done,
		Artifacts:   []string{legacyVPath},
	}); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Seed an intentionally stale row as if produced by old indexing behavior.
	if _, err := db.ExecContext(ctx, `DELETE FROM artifacts WHERE task_id = ?`, taskID); err != nil {
		t.Fatalf("delete artifacts: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO artifacts (task_id, team_id, run_id, role, task_kind, is_summary, display_name, vpath, disk_path, produced_at, day_bucket)
		VALUES (?, '', ?, 'unassigned', 'task', 1, 'SUMMARY.md', ?, ?, ?, ?)
	`, taskID, childRun.RunID, legacyVPath, filepath.Join(dir, "agents", childRun.RunID, "tasks", "2026-02-17", taskID, "SUMMARY.md"), done.Format(time.RFC3339Nano), "2026-02-17"); err != nil {
		t.Fatalf("insert stale artifact: %v", err)
	}

	if err := s.backfillArtifactIndex(ctx, db); err != nil {
		t.Fatalf("backfillArtifactIndex: %v", err)
	}

	rows, err := s.ListArtifactsByTask(ctx, ArtifactFilter{TaskID: taskID})
	if err != nil {
		t.Fatalf("ListArtifactsByTask: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 canonical row after backfill, got %d", len(rows))
	}
	wantVPath := "/tasks/subagent-1/2026-02-17/" + taskID + "/SUMMARY.md"
	if got := rows[0].VPath; got != wantVPath {
		t.Fatalf("vpath = %q, want %q", got, wantVPath)
	}
	wantDisk := filepath.Join(dir, "agents", childRun.ParentRunID, "tasks", "subagent-1", "2026-02-17", taskID, "SUMMARY.md")
	if got := rows[0].DiskPath; got != wantDisk {
		t.Fatalf("disk path = %q, want %q", got, wantDisk)
	}
	if got := rows[0].RunID; got != childRun.ParentRunID {
		t.Fatalf("run_id = %q, want parent run %q", got, childRun.ParentRunID)
	}
}
