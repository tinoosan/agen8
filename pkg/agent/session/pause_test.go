package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/fsutil"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/profile"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

type fakeAgent struct {
	mu         sync.Mutex
	runCount   int
	runStarted chan struct{}
	cfg        agent.AgentConfig
}

func newFakeAgent() *fakeAgent {
	return &fakeAgent{
		runStarted: make(chan struct{}, 32),
		cfg:        agent.AgentConfig{Model: "fake-model", Hooks: agent.Hooks{}},
	}
}

func (f *fakeAgent) Run(_ context.Context, _ string) (agent.RunResult, error) {
	f.mu.Lock()
	f.runCount++
	f.mu.Unlock()
	select {
	case f.runStarted <- struct{}{}:
	default:
	}
	return agent.RunResult{Text: "ok", Status: types.TaskStatusSucceeded}, nil
}

func (f *fakeAgent) RunConversation(_ context.Context, _ []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	return agent.RunResult{Text: "ok", Status: types.TaskStatusSucceeded}, nil, 0, nil
}

func (f *fakeAgent) ExecHostOp(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{Ok: true}
}

func (f *fakeAgent) GetModel() string                            { return f.cfg.Model }
func (f *fakeAgent) SetModel(v string)                           { f.cfg.Model = v }
func (f *fakeAgent) WebSearchEnabled() bool                      { return f.cfg.EnableWebSearch }
func (f *fakeAgent) SetEnableWebSearch(v bool)                   { f.cfg.EnableWebSearch = v }
func (f *fakeAgent) GetApprovalsMode() string                    { return f.cfg.ApprovalsMode }
func (f *fakeAgent) SetApprovalsMode(v string)                   { f.cfg.ApprovalsMode = v }
func (f *fakeAgent) GetReasoningEffort() string                  { return f.cfg.ReasoningEffort }
func (f *fakeAgent) SetReasoningEffort(v string)                 { f.cfg.ReasoningEffort = v }
func (f *fakeAgent) GetReasoningSummary() string                 { return f.cfg.ReasoningSummary }
func (f *fakeAgent) SetReasoningSummary(v string)                { f.cfg.ReasoningSummary = v }
func (f *fakeAgent) GetSystemPrompt() string                     { return f.cfg.SystemPrompt }
func (f *fakeAgent) SetSystemPrompt(v string)                    { f.cfg.SystemPrompt = v }
func (f *fakeAgent) GetHooks() *agent.Hooks                      { return &f.cfg.Hooks }
func (f *fakeAgent) SetHooks(v agent.Hooks)                      { f.cfg.Hooks = v }
func (f *fakeAgent) GetToolRegistry() agent.ToolRegistryProvider { return nil }
func (f *fakeAgent) SetToolRegistry(agent.ToolRegistryProvider)  {}
func (f *fakeAgent) GetExtraTools() []llmtypes.Tool              { return f.cfg.ExtraTools }
func (f *fakeAgent) SetExtraTools(v []llmtypes.Tool)             { f.cfg.ExtraTools = v }
func (f *fakeAgent) Clone() agent.Agent {
	return f
}
func (f *fakeAgent) Config() agent.AgentConfig { return f.cfg }
func (f *fakeAgent) CloneWithConfig(cfg agent.AgentConfig) (agent.Agent, error) {
	f.cfg = cfg
	return f, nil
}

func (f *fakeAgent) runs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runCount
}

func (f *fakeAgent) waitForRun(timeout time.Duration) bool {
	select {
	case <-f.runStarted:
		return true
	case <-time.After(timeout):
		return false
	}
}

type blockingAgent struct {
	cfg agent.AgentConfig
}

func newBlockingAgent() *blockingAgent {
	return &blockingAgent{cfg: agent.AgentConfig{Model: "fake-model", Hooks: agent.Hooks{}}}
}

func (b *blockingAgent) Run(ctx context.Context, _ string) (agent.RunResult, error) {
	<-ctx.Done()
	return agent.RunResult{}, ctx.Err()
}

func (b *blockingAgent) RunConversation(_ context.Context, _ []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	return agent.RunResult{Text: "ok", Status: types.TaskStatusSucceeded}, nil, 0, nil
}

func (b *blockingAgent) ExecHostOp(_ context.Context, _ types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{Ok: true}
}

func (b *blockingAgent) GetModel() string                            { return b.cfg.Model }
func (b *blockingAgent) SetModel(v string)                           { b.cfg.Model = v }
func (b *blockingAgent) WebSearchEnabled() bool                      { return b.cfg.EnableWebSearch }
func (b *blockingAgent) SetEnableWebSearch(v bool)                   { b.cfg.EnableWebSearch = v }
func (b *blockingAgent) GetApprovalsMode() string                    { return b.cfg.ApprovalsMode }
func (b *blockingAgent) SetApprovalsMode(v string)                   { b.cfg.ApprovalsMode = v }
func (b *blockingAgent) GetReasoningEffort() string                  { return b.cfg.ReasoningEffort }
func (b *blockingAgent) SetReasoningEffort(v string)                 { b.cfg.ReasoningEffort = v }
func (b *blockingAgent) GetReasoningSummary() string                 { return b.cfg.ReasoningSummary }
func (b *blockingAgent) SetReasoningSummary(v string)                { b.cfg.ReasoningSummary = v }
func (b *blockingAgent) GetSystemPrompt() string                     { return b.cfg.SystemPrompt }
func (b *blockingAgent) SetSystemPrompt(v string)                    { b.cfg.SystemPrompt = v }
func (b *blockingAgent) GetHooks() *agent.Hooks                      { return &b.cfg.Hooks }
func (b *blockingAgent) SetHooks(v agent.Hooks)                      { b.cfg.Hooks = v }
func (b *blockingAgent) GetToolRegistry() agent.ToolRegistryProvider { return nil }
func (b *blockingAgent) SetToolRegistry(agent.ToolRegistryProvider)  {}
func (b *blockingAgent) GetExtraTools() []llmtypes.Tool              { return b.cfg.ExtraTools }
func (b *blockingAgent) SetExtraTools(v []llmtypes.Tool)             { b.cfg.ExtraTools = v }
func (b *blockingAgent) Clone() agent.Agent                          { return b }
func (b *blockingAgent) Config() agent.AgentConfig                   { return b.cfg }
func (b *blockingAgent) CloneWithConfig(cfg agent.AgentConfig) (agent.Agent, error) {
	b.cfg = cfg
	return b, nil
}

type captureEventEmitter struct {
	mu     sync.Mutex
	events []events.Event
}

func (c *captureEventEmitter) Emit(_ context.Context, ev events.Event) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

func (c *captureEventEmitter) firstByType(kind string) (events.Event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ev := range c.events {
		if ev.Type == kind {
			return ev, true
		}
	}
	return events.Event{}, false
}

func waitUntil(t *testing.T, timeout, pollInterval time.Duration, cond func() bool, failure string) {
	t.Helper()
	if cond() {
		return
	}
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-timeoutTimer.C:
			t.Fatal(failure)
		case <-ticker.C:
			if cond() {
				return
			}
		}
	}
}

func assertNoPendingTasksDuring(t *testing.T, ts state.TaskStore, runID string, duration time.Duration, failure string) {
	t.Helper()
	timeoutTimer := time.NewTimer(duration)
	defer timeoutTimer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeoutTimer.C:
			return
		case <-ticker.C:
			count, err := ts.CountTasks(context.Background(), state.TaskFilter{
				RunID:  runID,
				Status: []types.TaskStatus{types.TaskStatusPending},
			})
			if err != nil {
				t.Fatalf("CountTasks: %v", err)
			}
			if count != 0 {
				t.Fatalf("%s: %d", failure, count)
			}
		}
	}
}

func TestSessionPausedSkipsPendingUntilResumed(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)

	runID := "run-test-1"
	sessionID := "sess-test-1"
	now := time.Now().UTC()
	if err := taskSvc.CreateTask(context.Background(), types.Task{
		TaskID:         "task-1",
		SessionID:      sessionID,
		RunID:          runID,
		AssignedToType: "agent",
		AssignedTo:     runID,
		Goal:           "do work",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	wakeCh := make(chan struct{}, 1)
	ag := newFakeAgent()
	sess, err := New(Config{
		Agent:        ag,
		Profile:      &profile.Profile{ID: "general"},
		TaskStore:    ts,
		SessionID:    sessionID,
		RunID:        runID,
		PollInterval: 20 * time.Millisecond,
		WakeCh:       wakeCh,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sess.SetPaused(true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sess.Run(ctx) }()

	// Explicitly nudge the loop while paused; it must not execute tasks.
	select {
	case wakeCh <- struct{}{}:
	default:
	}
	if ag.waitForRun(200 * time.Millisecond) {
		t.Fatalf("agent executed task while paused")
	}

	sess.SetPaused(false)
	select {
	case wakeCh <- struct{}{}:
	default:
	}

	if !ag.waitForRun(2 * time.Second) {
		t.Fatalf("agent did not resume task processing")
	}

	cancel()
	<-done
}

func TestHeartbeatEventsIncludeIntervalAndSource(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	em := &captureEventEmitter{}
	runID := "run-heartbeat-events"
	sessionID := "sess-heartbeat-events"
	sess, err := New(Config{
		Agent:        newFakeAgent(),
		Profile:      &profile.Profile{ID: "general"},
		TaskStore:    ts,
		SessionID:    sessionID,
		RunID:        runID,
		InstanceID:   runID,
		PollInterval: 20 * time.Millisecond,
		Events:       em,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	job := profile.HeartbeatJob{Name: "pulse", Goal: "heartbeat", Interval: time.Hour}
	sess.handleHeartbeat(context.Background(), job)

	pending, err := ts.ListTasks(context.Background(), state.TaskFilter{
		RunID:    runID,
		Status:   []types.TaskStatus{types.TaskStatusPending},
		TaskKind: state.TaskKindHeartbeat,
		Limit:    10,
		SortBy:   "created_at",
		SortDesc: true,
	})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending heartbeat tasks = %d want 1", len(pending))
	}
	if err := sess.runTask(context.Background(), pending[0].TaskID, pending[0]); err != nil {
		t.Fatalf("runTask: %v", err)
	}

	hbQueued, ok := em.firstByType("task.heartbeat.enqueued")
	if !ok {
		t.Fatalf("expected task.heartbeat.enqueued event")
	}
	if got := hbQueued.Data["interval"]; got != "1h0m0s" {
		t.Fatalf("heartbeat interval=%q want %q", got, "1h0m0s")
	}

	start, ok := em.firstByType("task.start")
	if !ok {
		t.Fatalf("expected task.start event")
	}
	if got := start.Data["source"]; got != "heartbeat" {
		t.Fatalf("task.start source=%q want heartbeat", got)
	}
	if got := start.Data["taskKind"]; got != state.TaskKindHeartbeat {
		t.Fatalf("task.start kind=%q want %q", got, state.TaskKindHeartbeat)
	}

	done, ok := em.firstByType("task.done")
	if !ok {
		t.Fatalf("expected task.done event")
	}
	if got := done.Data["source"]; got != "heartbeat" {
		t.Fatalf("task.done source=%q want heartbeat", got)
	}
	if got := done.Data["taskKind"]; got != state.TaskKindHeartbeat {
		t.Fatalf("task.done kind=%q want %q", got, state.TaskKindHeartbeat)
	}
	if _, ok := em.firstByType("task.heartbeat.done"); !ok {
		t.Fatalf("expected task.heartbeat.done event")
	}
}

func TestSessionPausedSkipsHeartbeatEnqueue(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	ag := newFakeAgent()
	runID := "run-test-2"
	sessionID := "sess-test-2"
	sess, err := New(Config{
		Agent: ag,
		Profile: &profile.Profile{
			ID: "general",
			Heartbeat: profile.HeartbeatConfig{
				Jobs: []profile.HeartbeatJob{{
					Name:     "pulse",
					Goal:     "heartbeat",
					Interval: 30 * time.Millisecond,
				}},
			},
		},
		TaskStore:    ts,
		SessionID:    sessionID,
		RunID:        runID,
		InstanceID:   runID,
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sess.SetPaused(true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sess.Run(ctx) }()
	assertNoPendingTasksDuring(t, ts, runID, 200*time.Millisecond, "heartbeat tasks enqueued while paused")
	cancel()
	<-done
}

func TestSessionHeartbeatDisabled_NoEnqueue(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	ag := newFakeAgent()
	runID := "run-heartbeat-disabled"
	sessionID := "sess-heartbeat-disabled"
	disabled := false
	sess, err := New(Config{
		Agent: ag,
		Profile: &profile.Profile{
			ID: "general",
			Heartbeat: profile.HeartbeatConfig{
				Enabled: &disabled,
				Jobs: []profile.HeartbeatJob{{
					Name:     "pulse",
					Goal:     "heartbeat",
					Interval: 30 * time.Millisecond,
				}},
			},
		},
		TaskStore:    ts,
		SessionID:    sessionID,
		RunID:        runID,
		InstanceID:   runID,
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sess.Run(ctx) }()
	assertNoPendingTasksDuring(t, ts, runID, 200*time.Millisecond, "heartbeat_enabled=false: expected no heartbeat tasks, got")
	cancel()
	<-done
}

func TestSessionContextCanceled_RecordsTaskCanceled(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	taskSvc := pkgtask.NewManager(ts, nil)

	runID := "run-test-cancel"
	sessionID := "sess-test-cancel"
	now := time.Now().UTC()
	if err := taskSvc.CreateTask(context.Background(), types.Task{
		TaskID:         "task-cancel",
		SessionID:      sessionID,
		RunID:          runID,
		AssignedToType: "agent",
		AssignedTo:     runID,
		Goal:           "long running task",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	sess, err := New(Config{
		Agent:        newBlockingAgent(),
		Profile:      &profile.Profile{ID: "general"},
		TaskStore:    ts,
		SessionID:    sessionID,
		RunID:        runID,
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sess.Run(ctx) }()

	waitUntil(t, 2*time.Second, 20*time.Millisecond, func() bool {
		task, gerr := ts.GetTask(context.Background(), "task-cancel")
		if gerr == nil && task.Status == types.TaskStatusActive {
			return true
		}
		return false
	}, "task did not become active before cancellation")
	cancel()
	<-done

	task, err := ts.GetTask(context.Background(), "task-cancel")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Status != types.TaskStatusCanceled {
		t.Fatalf("task status=%q want %q", task.Status, types.TaskStatusCanceled)
	}
}
