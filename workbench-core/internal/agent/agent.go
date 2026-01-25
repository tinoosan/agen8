package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
	"github.com/tinoosan/workbench-core/pkg/tools"
)

// HostExecutor is the host boundary for executing one host primitive.
//
// This interface exists so the agent can be reused outside the Workbench CLI:
// different hosts can implement execution over any environment as long as they
// support the HostOpRequest/HostOpResponse contract.
type HostExecutor interface {
	Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// HostExecFunc adapts a function to HostExecutor so hosts can implement
// primitive dispatchers by passing a standalone function.
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

// ContextSourceFunc adapts a function to ContextSource so callers can provide
// inline context selection logic when wiring up an agent.
type ContextSourceFunc func(ctx context.Context, basePrompt string, step int) (string, error)

func (f ContextSourceFunc) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	return f(ctx, basePrompt, step)
}

// Hooks are optional observability callbacks invoked by the agent loop.
type Hooks struct {
	// OnLLMUsage is invoked after each model call when token usage is available.
	OnLLMUsage func(step int, usage types.LLMUsage)

	// OnWebSearch is invoked after a model call when the provider returned URL citations.
	// This is used by the host UI to show when web-search grounding actually occurred.
	OnWebSearch func(step int, citations []types.LLMCitation)

	// OnToken is invoked for streamed output tokens (when the provider supports streaming).
	//
	// Phase 1: the agent loop emits only the decoded "final.text" stream.
	OnToken func(step int, text string)

	// OnStreamChunk is invoked for provider-level stream chunks that should not be
	// decoded/forwarded as user-visible output.
	//
	// Phase 2: used for "thinking" signals (reasoning progress + optional summary).
	OnStreamChunk func(step int, chunk types.LLMStreamChunk)

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

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants
	// when supported by the provider (e.g. OpenRouter ":online"). Host controls this.
	EnableWebSearch bool

	// PlanMode enforces the structured planning policy for the first step.
	PlanMode bool

	// ApprovalsMode controls whether the agent requires confirmation for sensitive ops.
	// Valid values are "enabled" (default) and "disabled". When enabled, host
	// primitives that are marked as requiring approval pause the loop and surface
	// a confirmation request via the host UI.
	ApprovalsMode string

	// ReasoningEffort is an optional hint for reasoning-capable models.
	// Accepted values are "none", "minimal", "low", "medium", "high",
	// or "xhigh" (provider best-effort). The agent forwards the hint to the LLM
	// client so it can request an appropriately detailed reasoning path.
	ReasoningEffort string

	// ReasoningSummary controls whether and how providers should emit reasoning summaries.
	// Accepted values are "off", "auto", "concise", or "detailed" (provider best-effort).
	ReasoningSummary string

	// SystemPrompt is the base system prompt to pass to the model.
	// If empty, the agent uses an internal default prompt.
	SystemPrompt string

	// Context optionally refreshes bounded context per model step.
	Context ContextSource

	// ToolManifests optionally supplies host-known tool manifests that should be
	// exposed as direct function tools (no discovery required).
	ToolManifests []tools.ToolManifest

	// MaxTokens restricts the output length. 0 means use client default.
	MaxTokens int

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

	system := strings.TrimSpace(cfg.SystemPrompt)
	if system == "" {
		system = agentLoopV0SystemPrompt()
	}

	extraTools, routes := ManifestToFunctionTools(cfg.ToolManifests)

	return &Agent{
		LLM:              cfg.LLM,
		Exec:             cfg.Exec,
		Model:            cfg.Model,
		EnableWebSearch:  cfg.EnableWebSearch,
		PlanMode:         cfg.PlanMode,
		ApprovalsMode:    strings.TrimSpace(cfg.ApprovalsMode),
		ReasoningEffort:  strings.TrimSpace(cfg.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(cfg.ReasoningSummary),
		SystemPrompt:     system,
		Context:          cfg.Context,
		MaxTokens:        cfg.MaxTokens,

		Hooks:              cfg.Hooks,
		ExtraTools:         extraTools,
		ToolFunctionRoutes: routes,
	}, nil
}
