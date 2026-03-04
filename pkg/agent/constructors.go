package agent

import (
	"fmt"
	"strings"

	hosttools "github.com/tinoosan/agen8/pkg/agent/hosttools"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/prompts"
)

// DefaultConfig returns the default agent configuration.
func DefaultConfig() AgentConfig {
	return AgentConfig{SystemPrompt: prompts.DefaultSystemPrompt()}
}

// NewAgent constructs an agent from explicit dependencies and configuration.
func NewAgent(llmClient llmtypes.LLMClient, exec HostExecutor, cfg AgentConfig) (Agent, error) {
	if llmClient == nil {
		return nil, errMissingLLM()
	}
	if exec == nil {
		return nil, errMissingExec()
	}
	useDefaultPrompt := strings.TrimSpace(cfg.SystemPrompt) == ""
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	registry, extraTools, err := registryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if useDefaultPrompt {
		cfg.SystemPrompt = prompts.DefaultSystemPromptWithTools(PromptToolSpecFromSources(registry, extraTools))
	}
	return &DefaultAgent{
		LLM:              llmClient,
		Exec:             exec,
		Model:            cfg.Model,
		EnableWebSearch:  cfg.EnableWebSearch,
		ApprovalsMode:    strings.TrimSpace(cfg.ApprovalsMode),
		ReasoningEffort:  strings.TrimSpace(cfg.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(cfg.ReasoningSummary),
		SystemPrompt:     strings.TrimSpace(cfg.SystemPrompt),
		PromptSource:     cfg.PromptSource,
		MaxTokens:        cfg.MaxTokens,

		Hooks:      cfg.Hooks,
		ExtraTools: extraTools,

		ToolRegistry: registry,
	}, nil
}

// NewDefaultAgent constructs a DefaultAgent from options.
func NewDefaultAgent(opts ...Option) (Agent, error) {
	cfg := &BuildConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return NewAgent(cfg.LLM, cfg.Exec, cfg.AgentConfig)
}

// DefaultHostToolRegistry returns a registry seeded with default tools.
func DefaultHostToolRegistry() (*HostToolRegistry, error) {
	registry := NewHostToolRegistry()
	for _, tool := range defaultHostTools() {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func registryFromConfig(cfg AgentConfig) (*HostToolRegistry, []llmtypes.Tool, error) {
	extraTools := append([]llmtypes.Tool(nil), cfg.ExtraTools...)
	if cfg.HostToolRegistry == nil {
		registry, err := DefaultHostToolRegistry()
		if err != nil {
			return nil, nil, err
		}
		return registry, extraTools, nil
	}

	registry := cfg.HostToolRegistry.Clone()
	if registry == nil {
		registry = NewHostToolRegistry()
	}
	return registry, extraTools, nil
}

func defaultHostTools() []HostTool {
	return []HostTool{
		&hosttools.FSListTool{},
		&hosttools.FSStatTool{},
		&hosttools.FSReadTool{},
		&hosttools.FSSearchTool{},
		&hosttools.FSWriteTool{},
		&hosttools.FSAppendTool{},
		&hosttools.FSEditTool{},
		&hosttools.FSPatchTool{},
		&hosttools.CodeExecTool{},
		&hosttools.ShellExecTool{},
		&hosttools.HTTPFetchTool{},
		&hosttools.EmailTool{},
		&hosttools.BrowserTool{},
		&hosttools.TraceRunTool{},
	}
}

func errMissingLLM() error {
	return fmt.Errorf("agent LLM is required")
}

func errMissingExec() error {
	return fmt.Errorf("agent Exec is required")
}
