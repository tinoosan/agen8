package agent

import (
	"fmt"
	"strings"

	agenttools "github.com/tinoosan/workbench-core/pkg/agent/tools"
	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/tools"
)

// DefaultConfig returns the default agent configuration.
func DefaultConfig() AgentConfig {
	return AgentConfig{SystemPrompt: DefaultSystemPrompt()}
}

// WorkerConfig returns the worker-focused configuration.
func WorkerConfig() AgentConfig {
	return AgentConfig{SystemPrompt: WorkerSystemPrompt()}
}

// NewAgent constructs an agent from explicit dependencies and configuration.
func NewAgent(llmClient llm.LLMClient, exec HostExecutor, cfg AgentConfig) (Agent, error) {
	if llmClient == nil {
		return nil, errMissingLLM()
	}
	if exec == nil {
		return nil, errMissingExec()
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = DefaultSystemPrompt()
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	registry, extraTools, err := registryFromConfig(cfg)
	if err != nil {
		return nil, err
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
		Context:          cfg.Context,
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
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = DefaultSystemPrompt()
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return NewAgent(cfg.LLM, cfg.Exec, cfg.AgentConfig)
}

// DefaultToolRegistry returns a registry seeded with default tools.
func DefaultToolRegistry(toolManifests []tools.ToolManifest) (*ToolRegistry, []llm.Tool, error) {
	return buildToolRegistry(toolManifests, defaultHostTools())
}

func registryFromConfig(cfg AgentConfig) (*ToolRegistry, []llm.Tool, error) {
	extraTools := append([]llm.Tool(nil), cfg.ExtraTools...)
	if cfg.ToolRegistry == nil {
		registry, manifestTools, err := DefaultToolRegistry(cfg.ToolManifests)
		if err != nil {
			return nil, nil, err
		}
		extraTools = append(extraTools, manifestTools...)
		return registry, extraTools, nil
	}

	registry := cfg.ToolRegistry.Clone()
	if registry == nil {
		registry = NewToolRegistry()
	}
	return registry, extraTools, nil
}

func buildToolRegistry(toolManifests []tools.ToolManifest, hostTools []HostTool) (*ToolRegistry, []llm.Tool, error) {
	extraTools, routes := ManifestToFunctionTools(toolManifests)
	registry := NewToolRegistry()
	registry.RegisterRoutes(routes)
	for _, tool := range hostTools {
		if err := registry.Register(tool); err != nil {
			return nil, nil, err
		}
	}
	return registry, extraTools, nil
}

func defaultHostTools() []HostTool {
	return []HostTool{
		&agenttools.FSListTool{},
		&agenttools.FSReadTool{},
		&agenttools.FSWriteTool{},
		&agenttools.FSAppendTool{},
		&agenttools.FSEditTool{},
		&agenttools.FSPatchTool{},
		&agenttools.ShellExecTool{},
		&agenttools.HTTPFetchTool{},
		&agenttools.ToolRunTool{},
		&agenttools.TraceEventsLatestTool{},
		&agenttools.TraceEventsSinceTool{},
		&agenttools.TraceEventsSummaryTool{},
	}
}

func errMissingLLM() error {
	return fmt.Errorf("agent LLM is required")
}

func errMissingExec() error {
	return fmt.Errorf("agent Exec is required")
}
