package agent

import (
	"sort"
	"strings"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/prompts"
)

const defaultFinalAnswerPromptDescription = "Emit the final response once the user's goal is complete."

// PromptToolSpecFromSources builds a normalized prompt tool spec from active registry + extra tools.
func PromptToolSpecFromSources(registry ToolRegistryProvider, extra []llmtypes.Tool) prompts.PromptToolSpec {
	byName := map[string]prompts.PromptTool{
		"final_answer": {Name: "final_answer", Description: defaultFinalAnswerPromptDescription},
	}
	add := func(name, description string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		desc := strings.TrimSpace(description)
		if existing, ok := byName[name]; ok {
			if strings.TrimSpace(existing.Description) != "" || desc == "" {
				return
			}
		}
		byName[name] = prompts.PromptTool{Name: name, Description: desc}
	}
	if registry != nil {
		for _, def := range registry.Definitions() {
			add(def.Function.Name, def.Function.Description)
		}
	}
	for _, def := range extra {
		add(def.Function.Name, def.Function.Description)
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]prompts.PromptTool, 0, len(names))
	for _, name := range names {
		tools = append(tools, byName[name])
	}
	return prompts.PromptToolSpec{Tools: tools}
}

// PromptToolSpecForCodeExecOnly builds prompt tool spec for code_exec_only mode.
// Model-visible tools come from modelRegistry; bridge guidance comes from bridgeRegistry.
func PromptToolSpecForCodeExecOnly(modelRegistry, bridgeRegistry ToolRegistryProvider, extra []llmtypes.Tool) prompts.PromptToolSpec {
	spec := PromptToolSpecFromSources(modelRegistry, extra)
	spec.CodeExecOnly = true
	spec.CodeExecBridgeTools = promptToolsFromRegistry(bridgeRegistry)
	return spec
}

func promptToolsFromRegistry(registry ToolRegistryProvider) []prompts.PromptTool {
	if registry == nil {
		return nil
	}
	byName := make(map[string]prompts.PromptTool)
	for _, def := range registry.Definitions() {
		name := strings.TrimSpace(def.Function.Name)
		if name == "" || strings.EqualFold(name, "final_answer") || strings.EqualFold(name, "code_exec") {
			continue
		}
		if _, ok := byName[name]; ok {
			continue
		}
		byName[name] = prompts.PromptTool{
			Name:        name,
			Description: strings.TrimSpace(def.Function.Description),
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]prompts.PromptTool, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

// SortedToolNamesFromRegistry returns deduplicated tool function names in lexical order.
func SortedToolNamesFromRegistry(registry ToolRegistryProvider) []string {
	if registry == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, def := range registry.Definitions() {
		name := strings.TrimSpace(def.Function.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
