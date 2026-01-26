package agent

import (
	"context"
	"fmt"
	"strings"

	agenttools "github.com/tinoosan/workbench-core/pkg/agent/tools"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// HostExecutor is the host boundary for executing one host primitive.
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
	OnLLMUsage    func(step int, usage llm.LLMUsage)
	OnWebSearch   func(step int, citations []llm.LLMCitation)
	OnToken       func(step int, text string)
	OnStreamChunk func(step int, chunk llm.LLMStreamChunk)
	Logf          func(format string, args ...any)
}

// Config configures a new Agent.
type Config struct {
	LLM llm.LLMClient

	// Exec is required and represents the host primitive dispatcher.
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-5-mini" (via OpenRouter), etc.
	Model string

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants.
	EnableWebSearch bool

	// PlanMode enforces the structured planning policy for the first step.
	PlanMode bool

	// ApprovalsMode controls whether the agent requires confirmation for sensitive ops.
	ApprovalsMode string

	// ReasoningEffort is an optional hint for reasoning-capable models.
	ReasoningEffort string

	// ReasoningSummary controls whether and how providers should emit reasoning summaries.
	ReasoningSummary string

	// SystemPrompt is the base system prompt to pass to the model.
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
	registry := NewToolRegistry()
	registry.RegisterRoutes(routes)

	registerTool := func(tool HostTool) error {
		if err := registry.Register(tool); err != nil {
			return err
		}
		return nil
	}
	if err := registerTool(&agenttools.FSListTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.FSReadTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.FSWriteTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.FSAppendTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.FSEditTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.FSPatchTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.UpdatePlanTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.ShellExecTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.HTTPFetchTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.ToolRunTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.TraceEventsLatestTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.TraceEventsSinceTool{}); err != nil {
		return nil, err
	}
	if err := registerTool(&agenttools.TraceEventsSummaryTool{}); err != nil {
		return nil, err
	}

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
		ToolRegistry:       registry,
	}, nil
}
