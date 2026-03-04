package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

// Ensure Manager implements interfaces at compile time.
var (
	_ state.TaskStore          = (*Manager)(nil)
	_ RetryEscalationCreator   = (*Manager)(nil)
	_ ActiveTaskCanceler       = (*Manager)(nil)
	_ ArtifactIndexerProvider  = (*Manager)(nil)
	_ TaskServiceForRPC        = (*Manager)(nil)
	_ TaskServiceForSupervisor = (*Manager)(nil)
	_ TaskServiceForTeam       = (*Manager)(nil)
)

type mockRunLoader struct {
	run types.Run
	err error
}

func (m *mockRunLoader) LoadRun(ctx context.Context, runID string) (types.Run, error) {
	if m.err != nil {
		return types.Run{}, m.err
	}
	return m.run, nil
}

type mockTaskStore struct {
	getTask      func(ctx context.Context, taskID string) (types.Task, error)
	listTasks    func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error)
	createTask   func(ctx context.Context, task types.Task) error
	completeTask func(ctx context.Context, taskID string, result types.TaskResult) error
	claimTask    func(ctx context.Context, taskID string, ttl time.Duration) error
	releaseLease func(ctx context.Context, taskID string) error
	delegateTask func(ctx context.Context, taskID string) error
	resumeTask   func(ctx context.Context, taskID string) error
}

type mockMessageStore struct {
	publishMessage   func(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error)
	claimNextMessage func(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error)
	ackMessage       func(ctx context.Context, messageID string, result state.MessageAckResult) error
	nackMessage      func(ctx context.Context, messageID string, reason string, retryAt *time.Time) error
	requeueExpired   func(ctx context.Context) error
	getMessage       func(ctx context.Context, messageID string) (types.AgentMessage, error)
	listMessages     func(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error)
	countMessages    func(ctx context.Context, filter state.MessageFilter) (int, error)
}

func (m *mockMessageStore) PublishMessage(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	if m.publishMessage != nil {
		return m.publishMessage(ctx, msg)
	}
	return msg, nil
}

func (m *mockMessageStore) ClaimNextMessage(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
	if m.claimNextMessage != nil {
		return m.claimNextMessage(ctx, filter, ttl, consumerID)
	}
	return types.AgentMessage{}, state.ErrMessageNotFound
}

func (m *mockMessageStore) AckMessage(ctx context.Context, messageID string, result state.MessageAckResult) error {
	if m.ackMessage != nil {
		return m.ackMessage(ctx, messageID, result)
	}
	return nil
}

func (m *mockMessageStore) NackMessage(ctx context.Context, messageID string, reason string, retryAt *time.Time) error {
	if m.nackMessage != nil {
		return m.nackMessage(ctx, messageID, reason, retryAt)
	}
	return nil
}

func (m *mockMessageStore) RequeueExpiredClaims(ctx context.Context) error {
	if m.requeueExpired != nil {
		return m.requeueExpired(ctx)
	}
	return nil
}

func (m *mockMessageStore) GetMessage(ctx context.Context, messageID string) (types.AgentMessage, error) {
	if m.getMessage != nil {
		return m.getMessage(ctx, messageID)
	}
	return types.AgentMessage{}, state.ErrMessageNotFound
}

func (m *mockMessageStore) ListMessages(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error) {
	if m.listMessages != nil {
		return m.listMessages(ctx, filter)
	}
	return nil, nil
}

func (m *mockMessageStore) CountMessages(ctx context.Context, filter state.MessageFilter) (int, error) {
	if m.countMessages != nil {
		return m.countMessages(ctx, filter)
	}
	return 0, nil
}

func (m *mockTaskStore) GetTask(ctx context.Context, taskID string) (types.Task, error) {
	if m.getTask != nil {
		return m.getTask(ctx, taskID)
	}
	return types.Task{}, state.ErrTaskNotFound
}

func (m *mockTaskStore) GetRunStats(ctx context.Context, runID string) (state.RunStats, error) {
	return state.RunStats{}, nil
}

func (m *mockTaskStore) ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
	if m.listTasks != nil {
		return m.listTasks(ctx, filter)
	}
	return nil, nil
}

func (m *mockTaskStore) CountTasks(ctx context.Context, filter state.TaskFilter) (int, error) {
	return 0, nil
}

func (m *mockTaskStore) CreateTask(ctx context.Context, task types.Task) error {
	if m.createTask != nil {
		return m.createTask(ctx, task)
	}
	return nil
}

func (m *mockTaskStore) DeleteTask(ctx context.Context, taskID string) error   { return nil }
func (m *mockTaskStore) UpdateTask(ctx context.Context, task types.Task) error { return nil }

func (m *mockTaskStore) CompleteTask(ctx context.Context, taskID string, result types.TaskResult) error {
	if m.completeTask != nil {
		return m.completeTask(ctx, taskID, result)
	}
	return nil
}

func (m *mockTaskStore) ClaimTask(ctx context.Context, taskID string, ttl time.Duration) error {
	if m.claimTask != nil {
		return m.claimTask(ctx, taskID, ttl)
	}
	return nil
}
func (m *mockTaskStore) ExtendLease(ctx context.Context, taskID string, ttl time.Duration) error {
	return nil
}
func (m *mockTaskStore) ReleaseLease(ctx context.Context, taskID string) error {
	if m.releaseLease != nil {
		return m.releaseLease(ctx, taskID)
	}
	return nil
}
func (m *mockTaskStore) DelegateTask(ctx context.Context, taskID string) error {
	if m.delegateTask != nil {
		return m.delegateTask(ctx, taskID)
	}
	return nil
}
func (m *mockTaskStore) ResumeTask(ctx context.Context, taskID string) error {
	if m.resumeTask != nil {
		return m.resumeTask(ctx, taskID)
	}
	return nil
}
func (m *mockTaskStore) RecoverExpiredLeases(ctx context.Context) error { return nil }

func TestManager_CreateRetryTask_NoRunLoader(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, nil)
	err := mgr.CreateRetryTask(context.Background(), "run-1", "feedback")
	if err == nil {
		t.Fatal("expected error when run loader is nil")
	}
	if !strings.Contains(err.Error(), "run loader not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_CreateRetryTask_EmptyChildRunID(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, &mockRunLoader{})
	err := mgr.CreateRetryTask(context.Background(), "", "feedback")
	if err == nil {
		t.Fatal("expected error for empty childRunID")
	}
}

func TestManager_CreateRetryTask_LoadRunError(t *testing.T) {
	loader := &mockRunLoader{err: errors.New("load failed")}
	mgr := NewManager(&mockTaskStore{}, loader)
	err := mgr.CreateRetryTask(context.Background(), "run-1", "feedback")
	if err == nil {
		t.Fatal("expected error when LoadRun fails")
	}
	if !errors.Is(err, loader.err) && !strings.Contains(err.Error(), "load failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestManager_CreateRetryTask_Success(t *testing.T) {
	run := types.Run{
		RunID:       "run-child",
		SessionID:   "sess-1",
		Goal:        "original goal",
		ParentRunID: "run-parent",
	}
	loader := &mockRunLoader{run: run}
	var created types.Task
	store := &mockTaskStore{
		createTask: func(ctx context.Context, task types.Task) error {
			created = task
			return nil
		},
	}
	mgr := NewManager(store, loader)
	err := mgr.CreateRetryTask(context.Background(), "run-child", "do it again")
	if err != nil {
		t.Fatalf("CreateRetryTask: %v", err)
	}
	if created.SessionID != "sess-1" || created.RunID != "run-child" {
		t.Errorf("created task session/run: %q / %q", created.SessionID, created.RunID)
	}
	if !strings.Contains(created.Goal, "do it again") || !strings.Contains(created.Goal, "original goal") {
		t.Errorf("goal: %s", created.Goal)
	}
	if created.Metadata["source"] != "retry" || created.Metadata["parentRunId"] != "run-parent" {
		t.Errorf("metadata: %+v", created.Metadata)
	}
}

func TestManager_CancelActiveTasksByRun_Fallback(t *testing.T) {
	active := map[string]struct{}{
		"t1": {},
		"t2": {},
	}
	var completed []string
	store := &mockTaskStore{
		listTasks: func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
			out := make([]types.Task, 0, len(active))
			for taskID := range active {
				out = append(out, types.Task{
					TaskID: taskID,
					RunID:  "run-1",
					Status: types.TaskStatusActive,
				})
			}
			return out, nil
		},
		completeTask: func(ctx context.Context, taskID string, result types.TaskResult) error {
			delete(active, taskID)
			completed = append(completed, taskID)
			return nil
		},
	}
	mgr := NewManager(store, nil)
	n, err := mgr.CancelActiveTasksByRun(context.Background(), "run-1", "paused")
	if err != nil {
		t.Fatalf("CancelActiveTasksByRun: %v", err)
	}
	if n != 2 {
		t.Errorf("count: got %d want 2", n)
	}
	if len(completed) != 2 || (completed[0] != "t1" && completed[0] != "t2") {
		t.Errorf("completed: %v", completed)
	}
}

func TestManager_CancelActiveTasksByRun_FallbackPaginatesBeyond500(t *testing.T) {
	active := make(map[string]struct{}, 505)
	for i := 0; i < 505; i++ {
		active[fmt.Sprintf("task-%03d", i)] = struct{}{}
	}
	var completed int
	store := &mockTaskStore{
		listTasks: func(ctx context.Context, filter state.TaskFilter) ([]types.Task, error) {
			tasks := make([]types.Task, 0, 500)
			for taskID := range active {
				if len(tasks) == 500 {
					break
				}
				tasks = append(tasks, types.Task{
					TaskID: taskID,
					RunID:  "run-1",
					Status: types.TaskStatusActive,
				})
			}
			return tasks, nil
		},
		completeTask: func(ctx context.Context, taskID string, result types.TaskResult) error {
			delete(active, taskID)
			completed++
			return nil
		},
	}
	mgr := NewManager(store, nil)
	n, err := mgr.CancelActiveTasksByRun(context.Background(), "run-1", "paused")
	if err != nil {
		t.Fatalf("CancelActiveTasksByRun: %v", err)
	}
	if n != 505 {
		t.Fatalf("count: got %d want 505", n)
	}
	if completed != 505 {
		t.Fatalf("completed: got %d want 505", completed)
	}
	if len(active) != 0 {
		t.Fatalf("remaining active tasks: %d", len(active))
	}
}

func TestManager_ArtifactIndexer_NotImplemented(t *testing.T) {
	mgr := NewManager(&mockTaskStore{}, nil)
	idx, ok := mgr.ArtifactIndexer()
	if ok || idx != nil {
		t.Errorf("ArtifactIndexer: want (nil, false), got (%v, %v)", idx, ok)
	}
}

func TestManager_CreateTask_CallbackRequiresTeamID(t *testing.T) {
	store := &mockTaskStore{
		createTask: func(ctx context.Context, task types.Task) error { return nil },
	}
	mgr := NewManager(store, nil)
	mgr.SetRoutingOracle(NewRoutingOracle())
	err := mgr.CreateTask(context.Background(), types.Task{
		TaskID:         "callback-task-1",
		SessionID:      "s1",
		RunID:          "run-parent",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "review",
		Status:         types.TaskStatusPending,
		Metadata:       map[string]any{"source": "subagent.callback"},
	})
	if err == nil || !strings.Contains(err.Error(), "missing teamId") {
		t.Fatalf("expected missing teamId error, got %v", err)
	}
}

func TestManager_CreateTask_CallbackInfersTeamIDFromRun(t *testing.T) {
	var created types.Task
	store := &mockTaskStore{
		createTask: func(ctx context.Context, task types.Task) error {
			created = task
			return nil
		},
	}
	loader := &mockRunLoader{
		run: types.Run{
			RunID: "run-parent",
			Runtime: &types.RunRuntimeConfig{
				TeamID: "team-1",
			},
		},
	}
	mgr := NewManager(store, loader)
	mgr.SetRoutingOracle(NewRoutingOracle())
	err := mgr.CreateTask(context.Background(), types.Task{
		TaskID:         "callback-task-2",
		SessionID:      "s1",
		RunID:          "run-parent",
		AssignedToType: "agent",
		AssignedTo:     "run-parent",
		Goal:           "review",
		Status:         types.TaskStatusPending,
		Metadata:       map[string]any{"source": "subagent.callback"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if got := strings.TrimSpace(created.TeamID); got != "team-1" {
		t.Fatalf("TeamID = %q, want team-1", got)
	}
	if created.Metadata["routingDecisionId"] == nil {
		t.Fatalf("expected routingDecisionId metadata")
	}
}

func TestManager_SubscribeWake_TriggeredByLifecycleMutations(t *testing.T) {
	taskID := "task-1"
	task := types.Task{TaskID: taskID, TeamID: "team-1", RunID: "run-1"}
	store := &mockTaskStore{
		getTask: func(ctx context.Context, id string) (types.Task, error) {
			if id != taskID {
				return types.Task{}, state.ErrTaskNotFound
			}
			return task, nil
		},
	}
	mgr := NewManager(store, nil)
	wakeCh, cancel := mgr.SubscribeWake("team-1", "run-1")
	defer cancel()

	expectWake := func(label string) {
		t.Helper()
		select {
		case <-wakeCh:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("expected wake for %s", label)
		}
	}

	if err := mgr.ClaimTask(context.Background(), taskID, time.Minute); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	expectWake("claim")

	if err := mgr.ReleaseLease(context.Background(), taskID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	expectWake("release lease")

	if err := mgr.DelegateTask(context.Background(), taskID); err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	expectWake("delegate")

	if err := mgr.ResumeTask(context.Background(), taskID); err != nil {
		t.Fatalf("ResumeTask: %v", err)
	}
	expectWake("resume")
}

func TestManager_ClaimTask_AllowsAlreadyClaimedMessageBySameRun(t *testing.T) {
	taskID := "task-1"
	task := types.Task{TaskID: taskID, SessionID: "sess-1", RunID: "run-1", Status: types.TaskStatusPending}
	claimedTask := task
	claimedTask.Status = types.TaskStatusActive
	var claimCalled bool
	store := &mockTaskStore{
		getTask: func(ctx context.Context, id string) (types.Task, error) {
			if id != taskID {
				return types.Task{}, state.ErrTaskNotFound
			}
			if claimCalled {
				return claimedTask, nil
			}
			return task, nil
		},
		claimTask: func(ctx context.Context, id string, ttl time.Duration) error {
			claimCalled = true
			return nil
		},
	}
	msgStore := &mockMessageStore{
		claimNextMessage: func(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
			return types.AgentMessage{}, state.ErrMessageNotFound
		},
		listMessages: func(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error) {
			return []types.AgentMessage{
				{
					MessageID:  "msg-1",
					TaskRef:    taskID,
					Status:     types.MessageStatusClaimed,
					LeaseOwner: "run-1",
				},
			}, nil
		},
	}
	mgr := NewManager(store, nil)
	mgr.SetMessageStore(msgStore)

	if err := mgr.ClaimTask(context.Background(), taskID, time.Minute); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if !claimCalled {
		t.Fatalf("expected task claim to be executed")
	}
}

func TestManager_ClaimTask_ReturnsMessageClaimedWhenOwnedByOtherRun(t *testing.T) {
	taskID := "task-2"
	task := types.Task{TaskID: taskID, SessionID: "sess-1", RunID: "run-1", Status: types.TaskStatusPending}
	var claimCalled bool
	store := &mockTaskStore{
		getTask: func(ctx context.Context, id string) (types.Task, error) {
			if id != taskID {
				return types.Task{}, state.ErrTaskNotFound
			}
			return task, nil
		},
		claimTask: func(ctx context.Context, id string, ttl time.Duration) error {
			claimCalled = true
			return nil
		},
	}
	msgStore := &mockMessageStore{
		claimNextMessage: func(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
			return types.AgentMessage{}, state.ErrMessageNotFound
		},
		listMessages: func(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error) {
			return []types.AgentMessage{
				{
					MessageID:  "msg-2",
					TaskRef:    taskID,
					Status:     types.MessageStatusClaimed,
					LeaseOwner: "run-other",
				},
			}, nil
		},
	}
	mgr := NewManager(store, nil)
	mgr.SetMessageStore(msgStore)

	err := mgr.ClaimTask(context.Background(), taskID, time.Minute)
	if !errors.Is(err, state.ErrMessageClaimed) {
		t.Fatalf("expected ErrMessageClaimed, got %v", err)
	}
	if claimCalled {
		t.Fatalf("did not expect task claim when backing message is owned by another run")
	}
}
