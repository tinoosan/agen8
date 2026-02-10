package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type fakeAgent struct {
	mu       sync.Mutex
	runCount int
	cfg      agent.AgentConfig
}

func newFakeAgent() *fakeAgent {
	return &fakeAgent{cfg: agent.AgentConfig{Model: "fake-model", Hooks: agent.Hooks{}}}
}

func (f *fakeAgent) Run(_ context.Context, _ string) (agent.RunResult, error) {
	f.mu.Lock()
	f.runCount++
	f.mu.Unlock()
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

func TestSessionPausedSkipsPendingUntilResumed(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	ts, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}

	runID := "run-test-1"
	sessionID := "sess-test-1"
	now := time.Now().UTC()
	if err := ts.CreateTask(context.Background(), types.Task{
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

	time.Sleep(80 * time.Millisecond)
	if got := ag.runs(); got != 0 {
		t.Fatalf("agent runs while paused = %d, want 0", got)
	}

	sess.SetPaused(false)
	wakeCh <- struct{}{}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ag.runs() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := ag.runs(); got == 0 {
		t.Fatalf("agent did not resume task processing")
	}

	cancel()
	<-done
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
			Heartbeat: []profile.HeartbeatJob{{
				Name:     "pulse",
				Goal:     "heartbeat",
				Interval: 30 * time.Millisecond,
			}},
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
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	count, err := ts.CountTasks(context.Background(), state.TaskFilter{
		RunID:  runID,
		Status: []types.TaskStatus{types.TaskStatusPending},
	})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("heartbeat tasks enqueued while paused: %d", count)
	}
}
