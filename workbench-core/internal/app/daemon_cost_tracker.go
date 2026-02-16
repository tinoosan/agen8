package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// CostTracker tracks usage and cost totals for a run/session pair.
type CostTracker interface {
	Track(step int, usage llmtypes.LLMUsage)
}

// SessionLoadSaver is the minimal session access needed by the cost tracker.
// Both store.SessionReaderWriter and pkgsession.Service satisfy it.
type SessionLoadSaver interface {
	LoadSession(ctx context.Context, sessionID string) (types.Session, error)
	SaveSession(ctx context.Context, s types.Session) error
}

type defaultCostTracker struct {
	cfg          config.Config
	run          types.Run
	modelID      string
	priceIn      float64
	priceOut     float64
	sessionStore SessionLoadSaver
	currentModel func() string
	emit         func(context.Context, events.Event)

	mu            sync.Mutex
	session       types.Session
	sessionLoaded bool
}

func newDefaultCostTracker(
	cfg config.Config,
	run types.Run,
	modelID string,
	priceIn float64,
	priceOut float64,
	sessionStore SessionLoadSaver,
	currentModel func() string,
	emit func(context.Context, events.Event),
) CostTracker {
	modelID = strings.TrimSpace(modelID)
	if currentModel == nil {
		currentModel = func() string { return modelID }
	}
	return &defaultCostTracker{
		cfg:          cfg,
		run:          run,
		modelID:      modelID,
		priceIn:      priceIn,
		priceOut:     priceOut,
		sessionStore: sessionStore,
		currentModel: currentModel,
		emit:         emit,
	}
}

func (t *defaultCostTracker) emitUsage(step, input, output, total, reasoning int) {
	if t == nil || t.emit == nil {
		return
	}
	// Use background context so usage events persist even if the run cancels.
	t.emit(context.Background(), events.Event{
		Type:    "llm.usage.total",
		Message: "LLM usage totals",
		Data: map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"input":     fmt.Sprintf("%d", input),
			"output":    fmt.Sprintf("%d", output),
			"total":     fmt.Sprintf("%d", total),
			"reasoning": fmt.Sprintf("%d", reasoning),
		},
	})
}

func (t *defaultCostTracker) emitCost(costUSD float64, known bool) {
	if t == nil || t.emit == nil {
		return
	}
	// Use background context so cost events persist even if the run cancels.
	payload := map[string]string{
		"known": fmt.Sprintf("%t", known),
	}
	if known {
		payload["costUSD"] = fmt.Sprintf("%.4f", costUSD)
	}
	t.emit(context.Background(), events.Event{
		Type:    "llm.cost.total",
		Message: "LLM cost totals",
		Data:    payload,
	})
}

func (t *defaultCostTracker) Track(step int, usage llmtypes.LLMUsage) {
	if t == nil {
		return
	}
	input := usage.InputTokens
	output := usage.OutputTokens
	total := usage.TotalTokens
	reasoning := usage.ReasoningTokens
	if total == 0 {
		total = input + output
	}

	model := strings.TrimSpace(t.currentModel())
	if model == "" {
		model = t.modelID
	}
	inPerM, outPerM, pricingKnown := resolvePricing(model, t.priceIn, t.priceOut)
	t.emitUsage(step, input, output, total, reasoning)

	costUSD := 0.0
	if pricingKnown {
		costUSD = (float64(input)/1_000_000.0)*inPerM + (float64(output)/1_000_000.0)*outPerM
	}
	t.emitCost(costUSD, pricingKnown)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.run.InputTokens += input
	t.run.OutputTokens += output
	t.run.TotalTokens += total
	if pricingKnown {
		t.run.CostUSD += costUSD
	}
	if err := implstore.SaveRun(t.cfg, t.run); err != nil {
		log.Printf("daemon: warning: failed to save run: %v", err)
		if t.emit != nil {
			t.emit(context.Background(), events.Event{
				Type:    "daemon.warning",
				Message: "Failed to persist run state",
				Data:    map[string]string{"error": err.Error()},
			})
		}
	}

	if !t.sessionLoaded {
		if t.sessionStore != nil {
			if s, err := t.sessionStore.LoadSession(context.Background(), t.run.SessionID); err == nil {
				t.session = s
				t.sessionLoaded = true
			}
		}
	}
	if t.sessionLoaded {
		t.session.InputTokens += input
		t.session.OutputTokens += output
		t.session.TotalTokens += total
		if pricingKnown {
			t.session.CostUSD += costUSD
		}
		if err := t.sessionStore.SaveSession(context.Background(), t.session); err != nil {
			log.Printf("daemon: warning: failed to save session: %v", err)
			if t.emit != nil {
				t.emit(context.Background(), events.Event{
					Type:    "daemon.warning",
					Message: "Failed to persist session state",
					Data:    map[string]string{"error": err.Error()},
				})
			}
		}
	}
}
