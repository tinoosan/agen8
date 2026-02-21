package app

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]types.Session
	runs     map[string]types.Run
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

func (m *memorySessionStore) SaveRun(_ context.Context, run types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs == nil {
		m.runs = map[string]types.Run{}
	}
	m.runs[run.RunID] = run
	return nil
}

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

func TestNewCostUsageHook_EmitsReasoningTokens(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	run := types.Run{RunID: "run-test-usage", SessionID: "sess-test-usage"}
	ss := &memorySessionStore{
		sessions: map[string]types.Session{
			run.SessionID: {SessionID: run.SessionID, ActiveModel: "openai/gpt-5-nano"},
		},
	}

	var mu sync.Mutex
	evs := []types.EventRecord{}
	emitFn := func(_ context.Context, ev events.Event) {
		mu.Lock()
		defer mu.Unlock()
		evs = append(evs, types.EventRecord{Type: ev.Type, Data: ev.Data})
	}

	hook := newCostUsageHook(cfg, run, "openai/gpt-5-nano", 0, 0, ss, func() string {
		return "openai/gpt-5-nano"
	}, emitFn)

	hook(2, llmtypes.LLMUsage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18, ReasoningTokens: 5})

	mu.Lock()
	defer mu.Unlock()
	if len(evs) == 0 {
		t.Fatalf("expected emitted events")
	}
	foundUsage := false
	for _, ev := range evs {
		if ev.Type != "llm.usage.total" {
			continue
		}
		foundUsage = true
		if got := strings.TrimSpace(ev.Data["reasoning"]); got != "5" {
			t.Fatalf("reasoning field = %q, want %q", got, "5")
		}
		if got := strings.TrimSpace(ev.Data["step"]); got != "2" {
			t.Fatalf("step field = %q, want %q", got, "2")
		}
	}
	if !foundUsage {
		t.Fatalf("expected llm.usage.total event")
	}
}
