package app

import (
	"fmt"

	"github.com/tinoosan/workbench-core/pkg/agent"
)

func registerAgentSpawnTool(registry *agent.HostToolRegistry, parentMaxTokens int, subagentModel string) error {
	if registry == nil {
		return fmt.Errorf("host tool registry is nil")
	}
	maxTokens := 0
	if parentMaxTokens > 1 {
		maxTokens = parentMaxTokens / 2
	}
	return registry.Register(&agent.AgentSpawnTool{
		MaxDepth:      3,
		CurrentDepth:  0,
		MaxTokens:     maxTokens,
		ModelOverride: subagentModel,
	})
}

func wireAgentSpawnParent(a agent.Agent) {
	if a == nil {
		return
	}
	reg, ok := a.GetToolRegistry().(*agent.HostToolRegistry)
	if !ok || reg == nil {
		return
	}
	tool, ok := reg.Get("agent_spawn")
	if !ok {
		return
	}
	spawn, ok := tool.(*agent.AgentSpawnTool)
	if !ok {
		return
	}
	spawn.ParentAgent = a
}
