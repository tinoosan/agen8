package agent

import (
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/tools"
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
	Context          ContextSource
	ToolManifests    []tools.ToolManifest
	ToolRegistry     *ToolRegistry
	ExtraTools       []llm.Tool
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
	LLM  llm.LLMClient
	Exec HostExecutor
	AgentConfig
}

// Option configures a concrete agent at construction time.
type Option func(*BuildConfig) error

// WithLLM sets the LLM client (required).
func WithLLM(c llm.LLMClient) Option {
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

func WithContextSource(src ContextSource) Option {
	return func(cfg *BuildConfig) error {
		cfg.Context = src
		return nil
	}
}

func WithToolManifests(m []tools.ToolManifest) Option {
	return func(cfg *BuildConfig) error {
		cfg.ToolManifests = m
		return nil
	}
}

func WithMaxTokens(n int) Option {
	return func(cfg *BuildConfig) error {
		cfg.MaxTokens = n
		return nil
	}
}

func WithToolRegistry(r *ToolRegistry) Option {
	return func(cfg *BuildConfig) error {
		cfg.ToolRegistry = r
		return nil
	}
}

func WithExtraTools(tools []llm.Tool) Option {
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
