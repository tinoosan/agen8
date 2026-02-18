package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
		SpawnIndex:  1,
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
		Artifacts: []string{"/workspace/output.md", "/tasks/2026-02-17/task-subagent-1/SUMMARY.md"},
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
	rawArtifacts, _ := callback.Inputs["artifacts"].([]any)
	artifacts := make([]string, 0, len(rawArtifacts))
	for _, a := range rawArtifacts {
		if s, ok := a.(string); ok {
			artifacts = append(artifacts, s)
		}
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d (%v)", len(artifacts), artifacts)
	}
	if artifacts[0] != "/workspace/subagent-1/output.md" {
		t.Fatalf("unexpected workspace artifact %q", artifacts[0])
	}
	if artifacts[1] != "/tasks/subagent-1/2026-02-17/task-subagent-1/SUMMARY.md" {
		t.Fatalf("unexpected summary artifact %q", artifacts[1])
	}
}

func TestNormalizeStandaloneSubagentCallbackArtifactPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "/workspace/report.md", want: "/workspace/subagent-2/report.md"},
		{in: "/workspace/subagent-2/report.md", want: "/workspace/subagent-2/report.md"},
		{in: "/tasks/2026-02-17/task-1/SUMMARY.md", want: "/tasks/subagent-2/2026-02-17/task-1/SUMMARY.md"},
		{in: "/tasks/subagent-2/2026-02-17/task-1/SUMMARY.md", want: "/tasks/subagent-2/2026-02-17/task-1/SUMMARY.md"},
	}
	for _, tc := range cases {
		if got := normalizeStandaloneSubagentCallbackArtifactPath(2, tc.in); got != tc.want {
			t.Fatalf("normalizeStandaloneSubagentCallbackArtifactPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaybeCreateCoordinatorCallback_SubagentBatchFlushesWhenComplete(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:   store,
		SessionID:   "session-parent",
		RunID:       "run-child-1",
		ParentRunID: "run-parent",
		SpawnIndex:  1,
	}}
	now := time.Now().UTC()
	for i := 1; i <= 3; i++ {
		taskID := fmt.Sprintf("task-subagent-%d", i)
		task := types.Task{
			TaskID:    taskID,
			SessionID: "session-parent",
			RunID:     "child-run",
			Goal:      "work",
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
			Metadata: map[string]any{
				"source":            "spawn_worker",
				"batchMode":         true,
				"batchParentTaskId": "task-parent-1",
				"parentRunId":       "run-parent",
			},
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask expected[%d]: %v", i, err)
		}
	}
	for i := 1; i <= 3; i++ {
		taskID := fmt.Sprintf("task-subagent-%d", i)
		task := types.Task{
			TaskID:    taskID,
			SessionID: "session-parent",
			RunID:     "child-run",
			Goal:      "work",
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
			Metadata: map[string]any{
				"source":            "spawn_worker",
				"batchMode":         true,
				"batchParentTaskId": "task-parent-1",
				"parentRunId":       "run-parent",
			},
		}
		s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
			TaskID:    task.TaskID,
			Status:    types.TaskStatusSucceeded,
			Summary:   "done",
			Artifacts: []string{"/workspace/output.md"},
		})
	}

	callbacks, err := store.ListTasks(context.Background(), state.TaskFilter{
		SessionID: "session-parent",
		Status:    []types.TaskStatus{types.TaskStatusReviewPending},
		SortBy:    "created_at",
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("ListTasks staged callbacks: %v", err)
	}
	if len(callbacks) < 3 {
		t.Fatalf("expected staged callbacks, got %d", len(callbacks))
	}
	batchCallbacks, err := store.ListTasks(context.Background(), state.TaskFilter{
		SessionID:      "session-parent",
		AssignedTo:     "run-parent",
		AssignedToType: "agent",
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		Limit:          50,
	})
	if err != nil {
		t.Fatalf("ListTasks pending: %v", err)
	}
	found := false
	for _, task := range batchCallbacks {
		loaded, _ := store.GetTask(context.Background(), task.TaskID)
		if strings.TrimSpace(fmt.Sprint(loaded.Metadata["source"])) == "subagent.batch.callback" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected one synthetic subagent batch callback")
	}
}

func TestMaybeCreateCoordinatorCallback_TeamBatchFlushesWhenComplete(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "workbench.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		TeamRoles:       []string{"ceo", "backend-engineer", "qa"},
		SessionID:       "team-team-1",
		RunID:           "run-backend",
	}}
	now := time.Now().UTC()
	for i := 1; i <= 2; i++ {
		taskID := fmt.Sprintf("task-team-%d", i)
		task := types.Task{
			TaskID:       taskID,
			SessionID:    "team-team-1",
			RunID:        "run-backend",
			TeamID:       "team-1",
			AssignedRole: "backend-engineer",
			CreatedBy:    "ceo",
			Goal:         "work",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata: map[string]any{
				"source":            "task_create",
				"batchMode":         true,
				"batchParentTaskId": "task-parent-team-1",
			},
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask expected[%d]: %v", i, err)
		}
	}
	for i := 1; i <= 2; i++ {
		taskID := fmt.Sprintf("task-team-%d", i)
		task := types.Task{
			TaskID:       taskID,
			SessionID:    "team-team-1",
			RunID:        "run-backend",
			TeamID:       "team-1",
			AssignedRole: "backend-engineer",
			CreatedBy:    "ceo",
			Goal:         "work",
			Status:       types.TaskStatusPending,
			CreatedAt:    &now,
			Metadata: map[string]any{
				"source":            "task_create",
				"batchMode":         true,
				"batchParentTaskId": "task-parent-team-1",
			},
		}
		s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
			TaskID:    task.TaskID,
			Status:    types.TaskStatusSucceeded,
			Summary:   "done",
			Artifacts: []string{"/workspace/backend-engineer/capabilities.md"},
		})
	}
	pending, err := store.ListTasks(context.Background(), state.TaskFilter{
		TeamID:         "team-1",
		AssignedRole:   "ceo",
		AssignedToType: "role",
		Status:         []types.TaskStatus{types.TaskStatusPending},
		SortBy:         "created_at",
		Limit:          50,
	})
	if err != nil {
		t.Fatalf("ListTasks pending: %v", err)
	}
	found := false
	for _, task := range pending {
		loaded, _ := store.GetTask(context.Background(), task.TaskID)
		if strings.TrimSpace(fmt.Sprint(loaded.Metadata["source"])) == "team.batch.callback" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected one synthetic team batch callback")
	}
}
