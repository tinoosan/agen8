package agent

import (
	"fmt"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// AgentConfig captures the configurable aspects of an agent.
type AgentConfig struct {
	Model            string
	EnableWebSearch  bool
	ApprovalsMode    string
	ReasoningEffort  string
	ReasoningSummary string
	SystemPrompt     string
	PromptSource     PromptSource
	HostToolRegistry *HostToolRegistry
	ExtraTools       []llmtypes.Tool
	MaxTokens        int
	Hooks            Hooks
}

func (cfg *AgentConfig) validate() error {
	if err := validate.NonEmpty("agent model", cfg.Model); err != nil {
		return err
	}
	return nil
}

// BuildConfig captures dependencies plus the agent configuration.
type BuildConfig struct {
	LLM  llmtypes.LLMClient
	Exec HostExecutor
	AgentConfig
}

// Option configures a concrete agent at construction time.
type Option func(*BuildConfig) error

// WithLLM sets the LLM client (required).
func WithLLM(c llmtypes.LLMClient) Option {
	return func(cfg *BuildConfig) error {
		cfg.LLM = c
		return nil
	}
}

// WithHostExecutor sets the host executor (required).
func WithHostExecutor(exec HostExecutor) Option {
	return func(cfg *BuildConfig) error {
		cfg.Exec = exec
		return nil
	}
}

// WithModel sets the model ID (required).
func WithModel(model string) Option {
	return func(cfg *BuildConfig) error {
		cfg.Model = strings.TrimSpace(model)
		return nil
	}
}

func WithWebSearch(enabled bool) Option {
	return func(cfg *BuildConfig) error {
		cfg.EnableWebSearch = enabled
		return nil
	}
}

func WithApprovalsMode(mode string) Option {
	return func(cfg *BuildConfig) error {
		cfg.ApprovalsMode = strings.TrimSpace(mode)
		return nil
	}
}

func WithReasoningEffort(effort string) Option {
	return func(cfg *BuildConfig) error {
		cfg.ReasoningEffort = strings.TrimSpace(effort)
		return nil
	}
}

func WithReasoningSummary(summary string) Option {
	return func(cfg *BuildConfig) error {
		cfg.ReasoningSummary = strings.TrimSpace(summary)
		return nil
	}
}

func WithSystemPrompt(prompt string) Option {
	return func(cfg *BuildConfig) error {
		cfg.SystemPrompt = strings.TrimSpace(prompt)
		return nil
	}
}

func WithPromptSource(src PromptSource) Option {
	return func(cfg *BuildConfig) error {
		cfg.PromptSource = src
		return nil
	}
}

func WithMaxTokens(n int) Option {
	return func(cfg *BuildConfig) error {
		cfg.MaxTokens = n
		return nil
	}
}

func WithHostToolRegistry(r *HostToolRegistry) Option {
	return func(cfg *BuildConfig) error {
		cfg.HostToolRegistry = r
		return nil
	}
}

func WithExtraTools(tools []llmtypes.Tool) Option {
	return func(cfg *BuildConfig) error {
		cfg.ExtraTools = tools
		return nil
	}
}

func WithHooks(h Hooks) Option {
	return func(cfg *BuildConfig) error {
		cfg.Hooks = h
		return nil
	}
}

func (cfg *BuildConfig) validate() error {
	if cfg.LLM == nil {
		return fmt.Errorf("agent LLM is required")
	}
	if cfg.Exec == nil {
		return fmt.Errorf("agent Exec is required")
	}
	if err := cfg.AgentConfig.validate(); err != nil {
		return err
	}
	return nil
}
