package agent

import (
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/tools"
)

// Config is the legacy configuration struct preserved for compatibility.
// New maps this struct to functional options.
type Config struct {
	LLM llm.LLMClient

	// Exec is required and represents the host primitive dispatcher.
	Exec HostExecutor

	// Model is required. Example: "openai/gpt-5-mini" (via OpenRouter), etc.
	Model string

	// EnableWebSearch controls whether the agent requests web-search-grounded model variants.
	EnableWebSearch bool

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

// New constructs a default Agent from the legacy Config for compatibility.
func New(cfg Config) (Agent, error) {
	agentCfg := AgentConfig{
		Model:            cfg.Model,
		EnableWebSearch:  cfg.EnableWebSearch,
		ApprovalsMode:    strings.TrimSpace(cfg.ApprovalsMode),
		ReasoningEffort:  strings.TrimSpace(cfg.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(cfg.ReasoningSummary),
		SystemPrompt:     cfg.SystemPrompt,
		Context:          cfg.Context,
		ToolManifests:    cfg.ToolManifests,
		MaxTokens:        cfg.MaxTokens,
		Hooks:            cfg.Hooks,
	}
	return NewAgent(cfg.LLM, cfg.Exec, agentCfg)
}
