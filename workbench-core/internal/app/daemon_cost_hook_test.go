package app

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/config"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]types.Session
}

func (m *memorySessionStore) LoadSession(_ context.Context, sessionID string) (types.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		return types.Session{}, nil
	}
	return m.sessions[sessionID], nil
}

func (m *memorySessionStore) SaveSession(_ context.Context, s types.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = map[string]types.Session{}
	}
	m.sessions[s.SessionID] = s
	return nil
}

var _ store.SessionReaderWriter = (*memorySessionStore)(nil)

func TestNewCostUsageHook_UsesCurrentModelPricing(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	run := types.Run{RunID: "run-test-cost", SessionID: "sess-test-cost"}
	modelA := "openai/gpt-5-nano"
	modelB := "openai/gpt-5-mini"
	currentModel := modelA

	ss := &memorySessionStore{
		sessions: map[string]types.Session{
			run.SessionID: {SessionID: run.SessionID, ActiveModel: modelA},
		},
	}

	hook := newCostUsageHook(cfg, run, modelA, 0, 0, ss, func() string {
		return currentModel
	}, nil)

	hook(1, llmtypes.LLMUsage{InputTokens: 1_000_000})
	currentModel = modelB
	hook(2, llmtypes.LLMUsage{OutputTokens: 1_000_000})

	sess, err := ss.LoadSession(context.Background(), run.SessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	want := 0.05 + 2.0 // gpt-5-nano input + gpt-5-mini output
	if math.Abs(sess.CostUSD-want) > 1e-9 {
		t.Fatalf("session cost mismatch: got %.10f want %.10f", sess.CostUSD, want)
	}
	if sess.InputTokens != 1_000_000 {
		t.Fatalf("session input tokens mismatch: got %d want %d", sess.InputTokens, 1_000_000)
	}
	if sess.OutputTokens != 1_000_000 {
		t.Fatalf("session output tokens mismatch: got %d want %d", sess.OutputTokens, 1_000_000)
	}
}
