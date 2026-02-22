package session

import (
	"context"
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

type mockRunConversationStore struct {
	data map[string][]llmtypes.LLMMessage
}

func (m *mockRunConversationStore) LoadMessages(ctx context.Context, runID string) ([]llmtypes.LLMMessage, error) {
	return m.data[runID], nil
}

func (m *mockRunConversationStore) SaveMessages(ctx context.Context, runID string, msgs []llmtypes.LLMMessage) error {
	if m.data == nil {
		m.data = make(map[string][]llmtypes.LLMMessage)
	}
	m.data[runID] = msgs
	return nil
}

type mockRunnerAgent struct {
	agent.Agent
	systemPrompt string
	config       agent.AgentConfig
}

func (m *mockRunnerAgent) GetSystemPrompt() string   { return m.systemPrompt }
func (m *mockRunnerAgent) Config() agent.AgentConfig { return m.config }
func (m *mockRunnerAgent) CloneWithConfig(cfg agent.AgentConfig) (agent.Agent, error) {
	clone := *m
	clone.systemPrompt = cfg.SystemPrompt
	clone.config = cfg
	return &clone, nil
}
func (m *mockRunnerAgent) GetModel() string { return "test-model" }

func (m *mockRunnerAgent) RunConversation(ctx context.Context, msgs []llmtypes.LLMMessage) (agent.RunResult, []llmtypes.LLMMessage, int, error) {
	updated := append([]llmtypes.LLMMessage(nil), msgs...)
	updated = append(updated, llmtypes.LLMMessage{
		Role:    "assistant",
		Content: "reply to " + msgs[len(msgs)-1].Content,
	})
	return agent.RunResult{Text: "reply", Status: types.TaskStatusSucceeded}, updated, 1, nil
}

func (m *mockRunnerAgent) Run(ctx context.Context, goal string) (agent.RunResult, error) {
	return agent.RunResult{Text: "run reply", Status: types.TaskStatusSucceeded}, nil
}

func (m *mockRunnerAgent) ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{Ok: true}
}

func TestRunConversationContext(t *testing.T) {
	ctx := context.Background()
	taskStore, _ := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	runConvStore := &mockRunConversationStore{data: make(map[string][]llmtypes.LLMMessage)}

	now := time.Now()
	task1 := types.Task{
		TaskID:         "t1",
		SessionID:      "sess1",
		RunID:          "run1",
		AssignedToType: "agent",
		AssignedTo:     "run1",
		TaskKind:       state.TaskKindTask,
		Goal:           "hello",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	_ = taskStore.CreateTask(ctx, task1)

	cfg := Config{
		Agent:                &mockRunnerAgent{},
		Profile:              &profile.Profile{ID: "test"},
		TaskStore:            taskStore,
		RunConversationStore: runConvStore,
		SessionID:            "sess1",
		RunID:                "run1",
		PollInterval:         50 * time.Millisecond,
		LeaseTTL:             1 * time.Minute,
		MaxRetries:           1,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Drain inbox manually (processes t1)
	_, _ = s.drainInbox(ctx)

	msgs := runConvStore.data["run1"]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after first task, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "reply to hello" {
		t.Errorf("unexpected messages: %+v", msgs)
	}

	// Create and drain second task
	task2 := types.Task{
		TaskID:         "t2",
		SessionID:      "sess1",
		RunID:          "run1",
		AssignedToType: "agent",
		AssignedTo:     "run1",
		TaskKind:       state.TaskKindTask,
		Goal:           "how are you",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	_ = taskStore.CreateTask(ctx, task2)
	_, _ = s.drainInbox(ctx)

	msgs = runConvStore.data["run1"]
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages after second task, got %d", len(msgs))
	}
	if msgs[2].Content != "how are you" || msgs[3].Content != "reply to how are you" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}

func TestRunConversationContext_NilStore(t *testing.T) {
	ctx := context.Background()
	taskStore, _ := state.NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))

	now := time.Now()
	task1 := types.Task{
		TaskID:         "t1",
		SessionID:      "sess1",
		RunID:          "run1",
		AssignedToType: "agent",
		AssignedTo:     "run1",
		TaskKind:       state.TaskKindTask,
		Goal:           "hello",
		Status:         types.TaskStatusPending,
		CreatedAt:      &now,
	}
	_ = taskStore.CreateTask(ctx, task1)

	cfg := Config{
		Agent:        &mockRunnerAgent{},
		Profile:      &profile.Profile{ID: "test"},
		TaskStore:    taskStore,
		SessionID:    "sess1",
		RunID:        "run1",
		PollInterval: 50 * time.Millisecond,
		LeaseTTL:     1 * time.Minute,
		MaxRetries:   1,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	_, _ = s.drainInbox(ctx)

	t1, _ := taskStore.GetTask(ctx, "t1")
	if !strings.Contains(t1.Summary, "run reply") {
		t.Errorf("expected Run() to be called (returning 'run reply'), got %q", t1.Summary)
	}
}
