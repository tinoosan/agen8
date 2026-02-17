package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestListPendingTasks_TeamRouting(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	create := func(task types.Task) {
		t.Helper()
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	create(types.Task{
		TaskID:       "task-worker",
		SessionID:    "s1",
		RunID:        "r1",
		TeamID:       "team-1",
		AssignedRole: "researcher",
		Goal:         "worker goal",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
	})
	create(types.Task{
		TaskID:       "task-coordinator",
		SessionID:    "s2",
		RunID:        "r2",
		TeamID:       "team-1",
		AssignedRole: "head-analyst",
		Goal:         "coordinator goal",
		Status:       types.TaskStatusPending,
		CreatedAt:    &now,
	})
	create(types.Task{
		TaskID:    "task-unassigned",
		SessionID: "s3",
		RunID:     "r3",
		TeamID:    "team-1",
		Goal:      "unassigned goal",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
	})

	worker := &Session{cfg: Config{
		TaskStore:  store,
		TeamID:     "team-1",
		RoleName:   "researcher",
		MaxPending: 50,
	}}
	workerTasks, err := worker.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("worker listPendingTasks: %v", err)
	}
	if len(workerTasks) != 1 || workerTasks[0].TaskID != "task-worker" {
		t.Fatalf("worker routing mismatch: %+v", workerTasks)
	}

	coordinator := &Session{cfg: Config{
		TaskStore:     store,
		TeamID:        "team-1",
		RoleName:      "head-analyst",
		IsCoordinator: true,
		MaxPending:    50,
	}}
	coordTasks, err := coordinator.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("coordinator listPendingTasks: %v", err)
	}
	if len(coordTasks) != 2 {
		t.Fatalf("expected coordinator to receive assigned+unassigned tasks, got %d", len(coordTasks))
	}
}

func TestNormalizeTeamCallbackArtifactPath(t *testing.T) {
	role := "frontend-engineer"
	cases := []struct {
		in   string
		want string
	}{
		{in: "/workspace/report.md", want: "/workspace/frontend-engineer/report.md"},
		{in: "/workspace/frontend-engineer/report.md", want: "/workspace/frontend-engineer/report.md"},
		{in: "/tasks/2026-02-17/task-1/SUMMARY.md", want: "/workspace/frontend-engineer/tasks/2026-02-17/task-1/SUMMARY.md"},
		{in: "/deliverables/r1/output.md", want: "/workspace/frontend-engineer/deliverables/r1/output.md"},
	}
	for _, tc := range cases {
		if got := normalizeTeamCallbackArtifactPath(role, tc.in); got != tc.want {
			t.Fatalf("normalizeTeamCallbackArtifactPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
