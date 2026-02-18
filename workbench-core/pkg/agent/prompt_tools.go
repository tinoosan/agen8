package agent

import (
	"sort"
	"strings"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/prompts"
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
