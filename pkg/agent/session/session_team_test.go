package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/agent/state"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestListPendingTasks_TeamRouting(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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

func TestClaimNextScopedMessage_TeamRoleFallsBackAcrossSessions(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	task := types.Task{
		TaskID:         "task-team-role-1",
		SessionID:      "sess-coordinator",
		RunID:          "run-coordinator",
		TeamID:         "team-1",
		AssignedRole:   "operations-lead",
		AssignedToType: "role",
		AssignedTo:     "operations-lead",
		Goal:           "delegate to ops",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	if err := store.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := store.PublishMessage(context.Background(), types.AgentMessage{
		MessageID:     "msg-team-role-1",
		IntentID:      "intent-team-role-1",
		CorrelationID: "corr-team-role-1",
		ThreadID:      "sess-coordinator",
		RunID:         "run-coordinator",
		TeamID:        "team-1",
		Channel:       types.MessageChannelInbox,
		Kind:          types.MessageKindTask,
		TaskRef:       task.TaskID,
		Task:          &task,
		Status:        types.MessageStatusPending,
		VisibleAt:     now,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}); err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}

	sess := &Session{cfg: Config{
		SessionID:  "sess-ops",
		RunID:      "run-ops",
		TeamID:     "team-1",
		RoleName:   "operations-lead",
		LeaseTTL:   time.Minute,
		MessageBus: store,
	}}

	msg, ok, err := sess.claimNextScopedMessage(context.Background(), sess.buildMessageClaimFilters())
	if err != nil {
		t.Fatalf("claimNextScopedMessage: %v", err)
	}
	if !ok {
		t.Fatalf("expected message claim across team sessions")
	}
	if got := strings.TrimSpace(msg.MessageID); got != "msg-team-role-1" {
		t.Fatalf("claimed message=%q want msg-team-role-1", got)
	}
	if got := strings.TrimSpace(msg.LeaseOwner); got != "run-ops" {
		t.Fatalf("leaseOwner=%q want run-ops", got)
	}
}

func TestProcessTaskMessage_SkipsStagedTeamCallbackMessages(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	task := types.Task{
		TaskID:         "callback-task-1",
		SessionID:      "team-team-1",
		RunID:          "run-reviewer",
		TeamID:         "team-1",
		AssignedRole:   "reviewer",
		AssignedToType: "role",
		AssignedTo:     "reviewer",
		TaskKind:       state.TaskKindCallback,
		Goal:           "single staged callback",
		Status:         types.TaskStatusReviewPending,
		CreatedAt:      &now,
		Metadata: map[string]any{
			"source":    "team.callback",
			"batchMode": true,
		},
	}
	if err := store.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	msg, err := store.PublishMessage(context.Background(), types.AgentMessage{
		MessageID:     "msg-callback-1",
		IntentID:      "intent-callback-1",
		CorrelationID: "corr-callback-1",
		ThreadID:      task.SessionID,
		RunID:         task.RunID,
		TeamID:        task.TeamID,
		Channel:       types.MessageChannelInbox,
		Kind:          types.MessageKindTask,
		TaskRef:       task.TaskID,
		Status:        types.MessageStatusPending,
		VisibleAt:     now,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})
	if err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}

	sess := &Session{cfg: Config{
		TaskStore:  store,
		MessageBus: store,
		SessionID:  task.SessionID,
		RunID:      "run-reviewer",
		TeamID:     task.TeamID,
		RoleName:   "reviewer",
		LeaseTTL:   time.Minute,
	}}
	if err := sess.processTaskMessage(context.Background(), msg); err != nil {
		t.Fatalf("processTaskMessage: %v", err)
	}

	loadedTask, err := store.GetTask(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loadedTask.Status != types.TaskStatusReviewPending {
		t.Fatalf("task status=%s want %s", loadedTask.Status, types.TaskStatusReviewPending)
	}
	loadedMsg, err := store.GetMessage(context.Background(), msg.MessageID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got := strings.TrimSpace(loadedMsg.Status); got != types.MessageStatusAcked {
		t.Fatalf("message status=%q want %q", got, types.MessageStatusAcked)
	}
}

func TestListPendingTasks_TeamChildRunUsesAgentAssignment(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	create := func(task types.Task) {
		t.Helper()
		if task.CreatedAt == nil {
			task.CreatedAt = &now
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	create(types.Task{
		TaskID:         "task-child-agent",
		SessionID:      "s1",
		RunID:          "run-child",
		TeamID:         "team-1",
		AssignedRole:   "Subagent-1",
		AssignedToType: "agent",
		AssignedTo:     "run-child",
		Goal:           "child task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	})
	create(types.Task{
		TaskID:         "task-role",
		SessionID:      "s1",
		RunID:          "run-parent",
		TeamID:         "team-1",
		AssignedRole:   "researcher",
		AssignedToType: "role",
		AssignedTo:     "researcher",
		Goal:           "role task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	})

	child := &Session{cfg: Config{
		TaskStore:   store,
		TeamID:      "team-1",
		RoleName:    "Subagent-1",
		RunID:       "run-child",
		ParentRunID: "run-parent",
		MaxPending:  50,
	}}
	tasks, err := child.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("child listPendingTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "task-child-agent" {
		t.Fatalf("expected child to receive only its agent-assigned task, got %+v", tasks)
	}
}

func TestListPendingTasks_TeamRoleIncludesAgentAddressedCallbacks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	create := func(task types.Task) {
		t.Helper()
		if task.CreatedAt == nil {
			task.CreatedAt = &now
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	// Normal team role task.
	create(types.Task{
		TaskID:         "task-role",
		SessionID:      "s1",
		RunID:          "run-parent",
		TeamID:         "team-1",
		AssignedRole:   "researcher",
		AssignedToType: "role",
		AssignedTo:     "researcher",
		Goal:           "role task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	})
	// Callback routed directly to the run (agent assignment).
	create(types.Task{
		TaskID:         "callback-task-role",
		SessionID:      "s1",
		RunID:          "run-parent",
		TeamID:         "team-1",
		AssignedRole:   "researcher",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "review spawned worker result",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Metadata:       map[string]any{"source": "subagent.callback"},
	})

	worker := &Session{cfg: Config{
		TaskStore:  store,
		TeamID:     "team-1",
		RoleName:   "researcher",
		RunID:      "run-parent",
		MaxPending: 50,
	}}
	tasks, err := worker.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("listPendingTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected role + agent-addressed callback tasks, got %d", len(tasks))
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		seen[strings.TrimSpace(task.TaskID)] = true
	}
	if !seen["task-role"] || !seen["callback-task-role"] {
		t.Fatalf("expected both tasks, got %+v", tasks)
	}
}

func TestListPendingTasks_TeamRoleSkipsLegacyReviewPendingCallbacks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	create := func(task types.Task) {
		t.Helper()
		if task.CreatedAt == nil {
			task.CreatedAt = &now
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	create(types.Task{
		TaskID:         "callback-review-pending",
		SessionID:      "s1",
		RunID:          "run-parent",
		TeamID:         "team-1",
		AssignedRole:   "researcher",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "review pending callback",
		Status:         types.TaskStatusReviewPending,
		Metadata:       map[string]any{"source": "subagent.callback"},
		CreatedAt:      &now,
	})
	create(types.Task{
		TaskID:         "non-callback-review-pending",
		SessionID:      "s1",
		RunID:          "run-parent",
		TeamID:         "team-1",
		AssignedRole:   "researcher",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "should not be scheduled",
		Status:         types.TaskStatusReviewPending,
		Metadata:       map[string]any{"source": "task_create"},
		CreatedAt:      &now,
	})

	worker := &Session{cfg: Config{
		TaskStore:  store,
		TeamID:     "team-1",
		RoleName:   "researcher",
		RunID:      "run-parent",
		MaxPending: 50,
	}}
	tasks, err := worker.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("listPendingTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no review_pending single callbacks, got %d (%+v)", len(tasks), tasks)
	}
}

func TestListPendingTasks_StandaloneParentIncludesBatchReviewPendingCallbacks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	create := func(task types.Task) {
		t.Helper()
		if task.CreatedAt == nil {
			task.CreatedAt = &now
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	create(types.Task{
		TaskID:         "task-normal-pending",
		SessionID:      "sess-parent",
		RunID:          "run-parent",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "normal",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	})
	create(types.Task{
		TaskID:         "callback-review-pending",
		SessionID:      "sess-parent",
		RunID:          "run-parent",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "callback",
		Status:         types.TaskStatusReviewPending,
		Metadata:       map[string]any{"source": "subagent.batch.callback"},
		CreatedAt:      &now,
	})

	parent := &Session{cfg: Config{
		TaskStore:  store,
		RunID:      "run-parent",
		SessionID:  "sess-parent",
		MaxPending: 50,
	}}
	tasks, err := parent.listPendingTasks(context.Background())
	if err != nil {
		t.Fatalf("listPendingTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected pending + batch callback review_pending, got %d (%+v)", len(tasks), tasks)
	}
	if strings.TrimSpace(tasks[0].TaskID) != "callback-review-pending" {
		t.Fatalf("expected callback task to be prioritized first, got %+v", tasks)
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
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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

func TestMaybeCreateCoordinatorCallback_TeamWorkerCompletion_AssignsReviewer(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		ReviewerRole:    "reviewer",
		TeamRoles:       []string{"ceo", "backend-engineer", "reviewer"},
		SessionID:       "team-team-1",
		RunID:           "run-backend",
	}}
	task := types.Task{
		TaskID:       "task-reviewer-route",
		TeamID:       "team-1",
		AssignedRole: "backend-engineer",
		CreatedBy:    "ceo",
		Goal:         "ship backend feature",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:  task.TaskID,
		Status:  types.TaskStatusSucceeded,
		Summary: "done",
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected callback task, got err=%v", err)
	}
	if callback.AssignedRole != "reviewer" {
		t.Fatalf("callback assignedRole=%q, want reviewer", callback.AssignedRole)
	}
}

func TestMaybeCreateCoordinatorCallback_ReviewerRoleCanBeOutsideTeamRoles(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		ReviewerRole:    "reviewer",
		TeamRoles:       []string{"ceo", "backend-engineer"},
		SessionID:       "team-team-1",
		RunID:           "run-backend",
	}}
	task := types.Task{
		TaskID:       "task-reviewer-fallback",
		TeamID:       "team-1",
		AssignedRole: "backend-engineer",
		CreatedBy:    "ceo",
		Goal:         "ship backend feature",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:  task.TaskID,
		Status:  types.TaskStatusSucceeded,
		Summary: "done",
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected callback task, got err=%v", err)
	}
	if callback.AssignedRole != "reviewer" {
		t.Fatalf("callback assignedRole=%q, want reviewer", callback.AssignedRole)
	}
}

func TestMaybeCreateCoordinatorCallback_SubagentWorkerCompletion_Unchanged(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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

func TestMaybeCreateCoordinatorCallback_SubagentWorkerCompletion_TeamContextSetsTeamID(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:   store,
		SessionID:   "session-child",
		RunID:       "run-child",
		ParentRunID: "run-parent",
		SpawnIndex:  1,
		TeamID:      "team-1",
	}}
	task := types.Task{
		TaskID:    "task-subagent-team-1",
		CreatedBy: "run-parent",
		Goal:      "do child work",
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:  task.TaskID,
		Status:  types.TaskStatusSucceeded,
		Summary: "child done",
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected subagent callback task, got err=%v", err)
	}
	if callback.TeamID != "team-1" {
		t.Fatalf("callback TeamID=%q, want team-1", callback.TeamID)
	}
}

func TestMaybeCreateCoordinatorCallback_TeamCallbackUsesWorkerSessionThread(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		ReviewerRole:    "reviewer",
		TeamRoles:       []string{"ceo", "reviewer", "backend-engineer"},
		SessionID:       "sess-team-1",
		RunID:           "run-backend",
	}}
	task := types.Task{
		TaskID:       "task-team-callback-1",
		SessionID:    "sess-team-1",
		RunID:        "run-backend",
		TeamID:       "team-1",
		AssignedRole: "backend-engineer",
		CreatedBy:    "ceo",
		Goal:         "deliver output",
		Status:       types.TaskStatusPending,
		Metadata: map[string]any{
			"source": "task_create",
		},
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:  task.TaskID,
		Status:  types.TaskStatusSucceeded,
		Summary: "done",
	})
	callback, err := store.GetTask(context.Background(), "callback-"+task.TaskID)
	if err != nil {
		t.Fatalf("expected team callback task, got err=%v", err)
	}
	if got := strings.TrimSpace(callback.SessionID); got != "sess-team-1" {
		t.Fatalf("callback SessionID=%q, want sess-team-1", got)
	}
	if got := strings.TrimSpace(callback.RunID); got != "run-backend" {
		t.Fatalf("callback RunID=%q, want run-backend", got)
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
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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

func TestMaybeCreateCoordinatorCallback_SubagentBatchWaitsUntilComplete(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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
	for i := 1; i <= 2; i++ {
		taskID := fmt.Sprintf("task-subagent-wait-%d", i)
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
				"batchParentTaskId": "task-parent-subagent-wait",
				"parentRunId":       "run-parent",
			},
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask expected[%d]: %v", i, err)
		}
	}

	completed := types.Task{
		TaskID:    "task-subagent-wait-1",
		SessionID: "session-parent",
		RunID:     "child-run",
		Goal:      "work",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			"source":            "spawn_worker",
			"batchMode":         true,
			"batchParentTaskId": "task-parent-subagent-wait",
			"parentRunId":       "run-parent",
		},
	}
	s.maybeCreateCoordinatorCallback(context.Background(), completed, types.TaskResult{
		TaskID:    completed.TaskID,
		Status:    types.TaskStatusSucceeded,
		Summary:   "done",
		Artifacts: []string{"/workspace/output.md"},
	})

	queued, err := store.ListTasks(context.Background(), state.TaskFilter{
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
	for _, task := range queued {
		loaded, _ := store.GetTask(context.Background(), task.TaskID)
		if strings.TrimSpace(fmt.Sprint(loaded.Metadata["source"])) == "subagent.batch.callback" {
			t.Fatalf("did not expect synthetic subagent batch callback before all callbacks are ready")
		}
	}

	staged, err := store.ListTasks(context.Background(), state.TaskFilter{
		SessionID: "session-parent",
		Status:    []types.TaskStatus{types.TaskStatusReviewPending},
		SortBy:    "created_at",
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("ListTasks staged callbacks: %v", err)
	}
	if len(staged) == 0 {
		t.Fatalf("expected staged callback task while waiting for full batch completion")
	}
}

func TestMaybeCreateCoordinatorCallback_TeamBatchFlushesWhenComplete(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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

func TestMaybeCreateCoordinatorCallback_TeamBatchWaitsUntilComplete(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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
		taskID := fmt.Sprintf("task-team-partial-%d", i)
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
				"batchParentTaskId": "task-parent-team-partial",
				"batchWaveId":       "wave-partial",
			},
		}
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask expected[%d]: %v", i, err)
		}
	}

	// Complete only one callback-producing task.
	// Batch should wait until all callbacks in the wave are staged.
	completedTask := types.Task{
		TaskID:       "task-team-partial-1",
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
			"batchParentTaskId": "task-parent-team-partial",
			"batchWaveId":       "wave-partial",
		},
	}
	s.maybeCreateCoordinatorCallback(context.Background(), completedTask, types.TaskResult{
		TaskID:    completedTask.TaskID,
		Status:    types.TaskStatusSucceeded,
		Summary:   "done",
		Artifacts: []string{"/workspace/backend-engineer/capabilities.md"},
	})

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
	for _, task := range pending {
		loaded, _ := store.GetTask(context.Background(), task.TaskID)
		if strings.TrimSpace(fmt.Sprint(loaded.Metadata["source"])) == "team.batch.callback" {
			t.Fatalf("did not expect synthetic team batch callback before all callbacks are ready")
		}
	}

	staged, err := store.ListTasks(context.Background(), state.TaskFilter{
		TeamID:   "team-1",
		Status:   []types.TaskStatus{types.TaskStatusReviewPending},
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    50,
	})
	if err != nil {
		t.Fatalf("ListTasks staged callbacks: %v", err)
	}
	foundStaged := false
	for _, task := range staged {
		loaded, _ := store.GetTask(context.Background(), task.TaskID)
		if strings.TrimSpace(fmt.Sprint(loaded.Metadata["source"])) == "team.callback" {
			foundStaged = true
			break
		}
	}
	if !foundStaged {
		t.Fatalf("expected staged team callback while waiting for full batch completion")
	}
}

func TestMaybeCreateCoordinatorCallback_SyntheticBatchDoesNotRequeueCallback(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
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
		TaskID:       "callback-batch-parent-1",
		TeamID:       "team-1",
		AssignedRole: "ceo",
		CreatedBy:    "backend-engineer",
		Goal:         "batch review",
		Metadata: map[string]any{
			"source":            "team.batch.callback",
			"batchMode":         true,
			"batchParentTaskId": "task-parent-1",
			"batchWaveId":       "wave-parent-1",
		},
	}
	s.maybeCreateCoordinatorCallback(context.Background(), task, types.TaskResult{
		TaskID:  task.TaskID,
		Status:  types.TaskStatusSucceeded,
		Summary: "reviewed",
	})
	_, err = store.GetTask(context.Background(), "callback-"+task.TaskID)
	if !errors.Is(err, state.ErrTaskNotFound) {
		t.Fatalf("expected no callback recursion, got err=%v", err)
	}
}

func TestMaybeCreateCoordinatorCallback_ReviewerBatchCompletionCreatesCoordinatorHandoff(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	child := types.Task{
		TaskID:         "callback-team-child",
		SessionID:      "team-team-1",
		RunID:          "team-team-1-callback",
		TeamID:         "team-1",
		AssignedRole:   "reviewer",
		AssignedToType: "role",
		AssignedTo:     "reviewer",
		TaskKind:       state.TaskKindCallback,
		Goal:           "review child",
		Status:         types.TaskStatusReviewPending,
		CreatedAt:      &now,
		Metadata: map[string]any{
			"source":            "team.callback",
			"callbackForTaskId": "task-child",
		},
	}
	if err := store.CreateTask(context.Background(), child); err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}
	batch := types.Task{
		TaskID:         "callback-batch-parent",
		SessionID:      "team-team-1",
		RunID:          "team-team-1-callback",
		TeamID:         "team-1",
		AssignedRole:   "reviewer",
		AssignedToType: "role",
		AssignedTo:     "reviewer",
		TaskKind:       state.TaskKindCallback,
		Goal:           "review batch",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
		Inputs: map[string]any{
			"items": []any{
				map[string]any{"callbackTaskId": "callback-team-child", "decision": "approve", "artifacts": []any{"/workspace/reviewer/review.md"}},
			},
		},
		Metadata: map[string]any{
			"source":             "team.batch.callback",
			"coordinatorRole":    "coordinator",
			"batchParentTaskId":  "task-parent",
			"batchWaveId":        "wave-a",
			"batchItemDecisions": map[string]any{"callback-team-child": "approve"},
		},
	}
	if err := store.CreateTask(context.Background(), batch); err != nil {
		t.Fatalf("CreateTask batch: %v", err)
	}

	s := &Session{
		cfg: Config{
			TaskStore:    store,
			TeamID:       "team-1",
			RoleName:     "reviewer",
			RunID:        "run-reviewer",
			SessionID:    "sess-reviewer",
			Events:       nil,
			TeamRoles:    []string{"coordinator", "reviewer"},
			ReviewerRole: "reviewer",
		},
	}
	tr := types.TaskResult{
		TaskID:      "callback-batch-parent",
		Status:      types.TaskStatusSucceeded,
		Summary:     "Review complete.",
		CompletedAt: &now,
		Artifacts:   []string{"/workspace/reviewer/review.md"},
	}
	s.maybeCreateCoordinatorCallback(context.Background(), batch, tr)

	handoff, err := store.GetTask(context.Background(), "review-handoff-callback-batch-parent")
	if err != nil {
		t.Fatalf("expected coordinator handoff task: %v", err)
	}
	if got := strings.TrimSpace(handoff.AssignedRole); got != "coordinator" {
		t.Fatalf("handoff assigned role=%q want coordinator", got)
	}
	if got := strings.TrimSpace(fmt.Sprint(handoff.Metadata["source"])); got != "review.handoff" {
		t.Fatalf("handoff source=%q want review.handoff", got)
	}
}

func TestMaybeCreateCoordinatorCallback_ReviewHandoffDoesNotRequeueReviewerCallback(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	now := time.Now().UTC()
	sess, err := New(Config{
		Agent: &errorAgent{cfg: agent.AgentConfig{Model: "fake-model", Hooks: agent.Hooks{}}},
		Profile: &profile.Profile{
			ID:          "team",
			Description: "team",
			Team: &profile.TeamConfig{
				Model: "gpt-5",
				Roles: []profile.RoleConfig{
					{Name: "coordinator", Coordinator: true, Description: "coord", Prompts: profile.PromptConfig{SystemPrompt: "coord"}},
					{Name: "reviewer", Description: "review", Prompts: profile.PromptConfig{SystemPrompt: "review"}},
				},
			},
		},
		TaskStore:         store,
		SessionID:         "sess-coord",
		RunID:             "run-coord",
		TeamID:            "team-1",
		RoleName:          "coordinator",
		IsCoordinator:     true,
		CoordinatorRole:   "coordinator",
		ReviewerRole:      "reviewer",
		TeamRoles:         []string{"coordinator", "reviewer"},
		LeaseTTL:          time.Minute,
		LeaseExtend:       30 * time.Second,
		PollInterval:      time.Second,
		MaxReadBytes:      1024,
		MaxPending:        50,
		MemorySearchLimit: 1,
		MaxRetries:        1,
		InstanceID:        "inst",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handoff := types.Task{
		TaskID:         "review-handoff-callback-batch-1",
		SessionID:      "sess-coord",
		RunID:          "run-coord",
		TeamID:         "team-1",
		AssignedRole:   "coordinator",
		AssignedToType: "role",
		AssignedTo:     "coordinator",
		TaskKind:       state.TaskKindCallback,
		Goal:           "REVIEW HANDOFF",
		Status:         types.TaskStatusSucceeded,
		CreatedAt:      &now,
		Metadata: map[string]any{
			"source":      "review.handoff",
			"batchTaskId": "callback-batch-1",
		},
	}
	sess.maybeCreateCoordinatorCallback(context.Background(), handoff, types.TaskResult{
		TaskID:      handoff.TaskID,
		Status:      types.TaskStatusSucceeded,
		CompletedAt: &now,
		Summary:     "done",
	})
	tasks, err := store.ListTasks(context.Background(), state.TaskFilter{
		TeamID:   "team-1",
		Status:   []types.TaskStatus{types.TaskStatusReviewPending, types.TaskStatusPending},
		SortBy:   "created_at",
		SortDesc: true,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == "callback-review-handoff-callback-batch-1" {
			t.Fatalf("unexpected callback generated from review.handoff completion")
		}
	}
}

func TestListBatchExpectedTasks_ExcludesSyntheticAndMatchesWave(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	s := &Session{cfg: Config{
		TaskStore:       store,
		TeamID:          "team-1",
		RoleName:        "backend-engineer",
		CoordinatorRole: "ceo",
		SessionID:       "team-team-1",
		RunID:           "run-backend",
	}}
	now := time.Now().UTC()
	mk := func(task types.Task) {
		t.Helper()
		if err := store.CreateTask(context.Background(), task); err != nil {
			t.Fatalf("CreateTask(%s): %v", task.TaskID, err)
		}
	}
	mk(types.Task{
		TaskID: "task-work-1", SessionID: "team-team-1", RunID: "run-backend", TeamID: "team-1", Goal: "work", Status: types.TaskStatusPending, CreatedAt: &now,
		Metadata: map[string]any{"source": "task_create", "batchMode": true, "batchParentTaskId": "task-parent-1", "batchWaveId": "wave-a"},
	})
	mk(types.Task{
		TaskID: "task-work-2", SessionID: "team-team-1", RunID: "run-backend", TeamID: "team-1", Goal: "work", Status: types.TaskStatusPending, CreatedAt: &now,
		Metadata: map[string]any{"source": "task_create", "batchMode": true, "batchParentTaskId": "task-parent-1", "batchWaveId": "wave-a"},
	})
	mk(types.Task{
		TaskID: "task-synthetic", SessionID: "team-team-1", RunID: "team-team-1-callback", TeamID: "team-1", Goal: "batch", Status: types.TaskStatusPending, CreatedAt: &now,
		Metadata: map[string]any{"source": "team.batch.callback", "batchMode": true, "batchSynthetic": true, "batchParentTaskId": "task-parent-1", "batchWaveId": "wave-a"},
	})
	mk(types.Task{
		TaskID: "task-other-wave", SessionID: "team-team-1", RunID: "run-backend", TeamID: "team-1", Goal: "work", Status: types.TaskStatusPending, CreatedAt: &now,
		Metadata: map[string]any{"source": "task_create", "batchMode": true, "batchParentTaskId": "task-parent-1", "batchWaveId": "wave-b"},
	})

	got := s.listBatchExpectedTasks(context.Background(), batchGroupScope{
		mode:         "team",
		parentTaskID: "task-parent-1",
		waveID:       "wave-a",
		reviewerID:   "ceo",
	})
	if len(got) != 2 {
		ids := make([]string, 0, len(got))
		for _, task := range got {
			ids = append(ids, task.TaskID)
		}
		t.Fatalf("expected 2 expected tasks in wave-a, got %d (%v)", len(got), ids)
	}
}

func TestSynthesizeBatchSummary_FillsEmptySummary(t *testing.T) {
	task := types.Task{
		Metadata: map[string]any{
			"source": "team.batch.callback",
			"batchItemDecisions": map[string]any{
				"callback-task-1": "approve",
				"callback-task-2": "retry",
			},
		},
		Inputs: map[string]any{
			"items": []any{
				map[string]any{"callbackTaskId": "callback-task-1", "sourceRole": "designer", "decision": "approve"},
				map[string]any{"callbackTaskId": "callback-task-2", "sourceRole": "finance-ops", "decision": "retry"},
			},
		},
	}
	got := synthesizeBatchSummary(task, types.TaskResult{Summary: ""})
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected synthesized summary")
	}
	if !strings.Contains(got, "approved=1") || !strings.Contains(got, "retry=1") {
		t.Fatalf("unexpected synthesized summary: %q", got)
	}
}

type errorAgent struct {
	err error
	cfg agent.AgentConfig
}

func (e *errorAgent) Run(context.Context, string) (agent.RunResult, error) {
	return agent.RunResult{}, e.err
}
func (e *errorAgent) RunConversation(context.Context, []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	return agent.RunResult{}, nil, 0, e.err
}
func (e *errorAgent) ExecHostOp(context.Context, types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{Ok: true}
}
func (e *errorAgent) GetModel() string                            { return e.cfg.Model }
func (e *errorAgent) SetModel(v string)                           { e.cfg.Model = v }
func (e *errorAgent) WebSearchEnabled() bool                      { return e.cfg.EnableWebSearch }
func (e *errorAgent) SetEnableWebSearch(v bool)                   { e.cfg.EnableWebSearch = v }
func (e *errorAgent) GetApprovalsMode() string                    { return e.cfg.ApprovalsMode }
func (e *errorAgent) SetApprovalsMode(v string)                   { e.cfg.ApprovalsMode = v }
func (e *errorAgent) GetReasoningEffort() string                  { return e.cfg.ReasoningEffort }
func (e *errorAgent) SetReasoningEffort(v string)                 { e.cfg.ReasoningEffort = v }
func (e *errorAgent) GetReasoningSummary() string                 { return e.cfg.ReasoningSummary }
func (e *errorAgent) SetReasoningSummary(v string)                { e.cfg.ReasoningSummary = v }
func (e *errorAgent) GetSystemPrompt() string                     { return e.cfg.SystemPrompt }
func (e *errorAgent) SetSystemPrompt(v string)                    { e.cfg.SystemPrompt = v }
func (e *errorAgent) GetHooks() *agent.Hooks                      { return &e.cfg.Hooks }
func (e *errorAgent) SetHooks(v agent.Hooks)                      { e.cfg.Hooks = v }
func (e *errorAgent) GetToolRegistry() agent.ToolRegistryProvider { return nil }
func (e *errorAgent) SetToolRegistry(agent.ToolRegistryProvider)  {}
func (e *errorAgent) GetExtraTools() []llmtypes.Tool              { return e.cfg.ExtraTools }
func (e *errorAgent) SetExtraTools(v []llmtypes.Tool)             { e.cfg.ExtraTools = v }
func (e *errorAgent) Clone() agent.Agent                          { return e }
func (e *errorAgent) Config() agent.AgentConfig                   { return e.cfg }
func (e *errorAgent) CloneWithConfig(cfg agent.AgentConfig) (agent.Agent, error) {
	e.cfg = cfg
	return e, nil
}

func TestRunTask_EmitsInvalidRepeatedEvent(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	emitter := &captureEventEmitter{}
	ag := &errorAgent{
		err: &agent.RepeatedInvalidToolCallError{
			ToolName:    "task_create",
			Count:       6,
			LastError:   "task_create.assignedRole is required for coordinators in team mode",
			Elapsed:     5 * time.Second,
			Coordinator: true,
		},
		cfg: agent.AgentConfig{Model: "fake-model", Hooks: agent.Hooks{}},
	}
	sess, err := New(Config{
		Agent:      ag,
		Profile:    &profile.Profile{ID: "startup_team"},
		TaskStore:  store,
		Events:     emitter,
		SessionID:  "session-1",
		RunID:      "run-1",
		TeamID:     "team-1",
		RoleName:   "ceo",
		MaxPending: 10,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	now := time.Now().UTC()
	task := types.Task{
		TaskID:         "task-invalid-1",
		SessionID:      "session-1",
		RunID:          "run-1",
		TeamID:         "team-1",
		AssignedRole:   "ceo",
		AssignedToType: "role",
		AssignedTo:     "ceo",
		Goal:           "delegate task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	if err := store.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := sess.runTask(context.Background(), task.TaskID, task); err != nil {
		t.Fatalf("runTask: %v", err)
	}
	ev, ok := emitter.firstByType("task.tool.invalid_repeated")
	if !ok {
		t.Fatalf("expected task.tool.invalid_repeated event")
	}
	if ev.Data["consecutiveInvalid"] != "6" {
		t.Fatalf("consecutiveInvalid=%q, want 6", ev.Data["consecutiveInvalid"])
	}
}
