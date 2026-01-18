package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// HostExecutor is the host boundary for executing one host primitive.
//
// This interface exists so the agent can be reused outside the Workbench CLI:
// different hosts can implement execution over any environment as long as they
// support the HostOpRequest/HostOpResponse contract.
type HostExecutor interface {
	Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// HostExecFunc adapts a function to HostExecutor.
type HostExecFunc func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse

func (f HostExecFunc) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return f(ctx, req)
}

// ContextSource produces an augmented system prompt per agent step.
//
// A ContextSource is responsible for selecting and injecting bounded context
// (memory, profile, trace summaries, etc). The agent calls it once per step.
type ContextSource interface {
	SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error)
}

// ContextSourceFunc adapts a function to ContextSource.
type ContextSourceFunc func(ctx context.Context, basePrompt string, step int) (string, error)

func (f ContextSourceFunc) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	return f(ctx, basePrompt, step)
}

// Hooks are optional observability callbacks invoked by the agent loop.
type Hooks struct {
	// OnLLMUsage is invoked after each model call when token usage is available.
	OnLLMUsage func(step int, usage types.LLMUsage)

	// Logf is an optional logger used to print what the agent is doing.
	Logf func(format string, args ...any)
}

// Config configures a new Agent.
type Config struct {
	LLM types.LLMClient

	// Exec is required and represents the host primitive dispatcher.
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-5-mini" (via OpenRouter), etc.
	Model string

	// SystemPrompt is the base system prompt to pass to the model.
	// If empty, the agent uses an internal default prompt.
	SystemPrompt string

	// Context optionally refreshes bounded context per model step.
	Context ContextSource

	// MaxSteps caps the number of model -> host op iterations per user turn.
	MaxSteps int

	Hooks Hooks
}

// New constructs an Agent from a validated config.
func New(cfg Config) (*Agent, error) {
	if cfg.LLM == nil {
		return nil, fmt.Errorf("agent LLM is required")
	}
	if cfg.Exec == nil {
		return nil, fmt.Errorf("agent Exec is required")
	}
	if err := validate.NonEmpty("agent Model", cfg.Model); err != nil {
		return nil, err
	}
	if err := validate.Positive("agent MaxSteps", cfg.MaxSteps); err != nil {
		return nil, err
	}

	system := strings.TrimSpace(cfg.SystemPrompt)
	if system == "" {
		system = agentLoopV0SystemPrompt()
	}

	return &Agent{
		LLM:          cfg.LLM,
		Exec:         cfg.Exec,
		Model:        cfg.Model,
		SystemPrompt: system,
		Context:      cfg.Context,
		MaxSteps:     cfg.MaxSteps,
		Hooks:        cfg.Hooks,
	}, nil
}
