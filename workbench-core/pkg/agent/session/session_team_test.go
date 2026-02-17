package session

import (
	"context"
	"errors"
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
	teamRoles := []string{"frontend-engineer", "qa-engineer"}
	cases := []struct {
		in   string
		want string
	}{
		{in: "/workspace/report.md", want: "/workspace/frontend-engineer/report.md"},
		{in: "/workspace/frontend-engineer/report.md", want: "/workspace/frontend-engineer/report.md"},
		{in: "/workspace/qa-engineer/report.md", want: "/workspace/qa-engineer/report.md"},
		{in: "/tasks/2026-02-17/task-1/SUMMARY.md", want: "/tasks/frontend-engineer/2026-02-17/task-1/SUMMARY.md"},
		{in: "/tasks/qa-engineer/2026-02-17/task-1/SUMMARY.md", want: "/tasks/qa-engineer/2026-02-17/task-1/SUMMARY.md"},
		{in: "/deliverables/r1/output.md", want: "/deliverables/r1/output.md"},
	}
	for _, tc := range cases {
		if got := normalizeTeamCallbackArtifactPath(role, teamRoles, tc.in); got != tc.want {
			t.Fatalf("normalizeTeamCallbackArtifactPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaybeCreateCoordinatorCallback_TeamCoordinatorSelfTask_NoCallback(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "ceo",
		CoordinatorRole: "ceo",
		IsCoordinator:   true,
		TeamRoles:       []string{"ceo", "backend-engineer"},
		SessionID:       "team-team-1",
		RunID:           "run-ceo",
	}}
	task := types.Task{
		TaskID:       "task-ceo-capabilities",
		TeamID:       "team-1",
		AssignedRole: "ceo",
		CreatedBy:    "user",
		Goal:         "Create CEO capabilities doc",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:    task.TaskID,
		Status:    types.TaskStatusSucceeded,
		Summary:   "done",
		Artifacts: []string{"/workspace/ceo/capabilities.md"},
	})
	_, err = store.GetTask(context.Background(), "callback-"+task.TaskID)
	if !errors.Is(err, state.ErrTaskNotFound) {
		t.Fatalf("expected no callback task for coordinator self-task, got err=%v", err)
	}
}

func TestMaybeCreateCoordinatorCallback_TeamWorkerCompletion_CreatesCallback(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		TeamRoles:       []string{"ceo", "backend-engineer"},
		SessionID:       "team-team-1",
		RunID:           "run-backend",
	}}
	task := types.Task{
		TaskID:       "task-backend-capabilities",
		TeamID:       "team-1",
		AssignedRole: "backend-engineer",
		CreatedBy:    "ceo",
		Goal:         "Create backend capabilities doc",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:    task.TaskID,
		Status:    types.TaskStatusSucceeded,
		Summary:   "done",
		Artifacts: []string{"/workspace/backend-engineer/capabilities.md"},
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected callback task, got err=%v", err)
	}
	if callback.AssignedRole != "ceo" {
		t.Fatalf("callback assignedRole=%q, want ceo", callback.AssignedRole)
	}
	if callback.TaskKind != state.TaskKindCallback {
		t.Fatalf("callback taskKind=%q, want %q", callback.TaskKind, state.TaskKindCallback)
	}
}

func TestMaybeCreateCoordinatorCallback_SubagentWorkerCompletion_Unchanged(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:   store,
		SessionID:   "session-child",
		RunID:       "run-child",
		ParentRunID: "run-parent",
	}}
	task := types.Task{
		TaskID:    "task-subagent-1",
		CreatedBy: "run-parent",
		Goal:      "do child work",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:    task.TaskID,
		Status:    types.TaskStatusSucceeded,
		Summary:   "child done",
		Artifacts: []string{"/deliverables/output.md", "/tasks/2026-02-17/task-subagent-1/SUMMARY.md"},
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected subagent callback task, got err=%v", err)
	}
	if callback.AssignedTo != "run-parent" {
		t.Fatalf("subagent callback assignedTo=%q, want run-parent", callback.AssignedTo)
	}
	if callback.Metadata["source"] != "subagent.callback" {
		t.Fatalf("subagent callback source=%v, want subagent.callback", callback.Metadata["source"])
	}
}
