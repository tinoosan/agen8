package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

// Config is the constructor configuration for Agent.
//
// This exists to keep host code (internal/app, cmd/workbench) lean:
// instead of building an Agent with a large struct literal, the host provides
// a small config bundle and the constructor applies defaults + validation.
type Config struct {
	LLM            types.LLMClient
	Exec           func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
	Model          string
	SystemPrompt   string
	ContextUpdater *ContextUpdater
	MaxSteps       int

	// Optional hooks (host observability).
	Logf       func(format string, args ...any)
	OnLLMUsage func(step int, usage types.LLMUsage)
}

// New constructs an Agent from a validated config.
func New(cfg Config) (*Agent, error) {
	if cfg.LLM == nil {
		return nil, fmt.Errorf("agent LLM is required")
	}
	if cfg.Exec == nil {
		return nil, fmt.Errorf("agent Exec is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("agent Model is required")
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		return nil, fmt.Errorf("agent SystemPrompt is required")
	}
	if cfg.MaxSteps <= 0 {
		return nil, fmt.Errorf("agent MaxSteps must be > 0")
	}

	return &Agent{
		LLM:            cfg.LLM,
		Exec:           cfg.Exec,
		Model:          cfg.Model,
		SystemPrompt:   cfg.SystemPrompt,
		ContextUpdater: cfg.ContextUpdater,
		MaxSteps:       cfg.MaxSteps,
		Logf:           cfg.Logf,
		OnLLMUsage:     cfg.OnLLMUsage,
	}, nil
}
