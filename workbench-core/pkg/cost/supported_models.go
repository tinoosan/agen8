package cost

import (
	"sort"
	"strings"
)

type ModelInfo struct {
	ID          string
	Provider    string
	InputPerM   float64
	OutputPerM  float64
	IsReasoning bool
}

var modelInfos = []ModelInfo{
	{"openai/gpt-5.2", "openai", 1.75, 14.0, true},
	{"openai/gpt-5.2-chat", "openai", 1.75, 14.0, true},
	{"openai/gpt-5.2-pro", "openai", 21.0, 168.0, true},
	{"openai/gpt-5.1", "openai", 1.25, 10.0, true},
	{"openai/gpt-5.1-chat", "openai", 1.25, 10.0, true},
	{"openai/gpt-5.1-codex", "openai", 1.25, 10.0, true},
	{"openai/gpt-5.1-codex-mini", "openai", 0.25, 2.0, true},
	{"openai/gpt-5.1-codex-max", "openai", 1.25, 10.0, true},
	{"openai/gpt-5-mini", "openai", 0.25, 2.0, true},
	{"openai/gpt-5-nano", "openai", 0.05, 0.4, true},
	{"openai/gpt-4.1", "openai", 0, 0, false},
	{"openai/gpt-4o", "openai", 2.5, 10.0, false},
	{"openai/gpt-4o-mini", "openai", 0.15, 0.6, false},
	{"openai/o1-preview", "openai", 15.0, 60.0, true},
	{"openai/o1-mini", "openai", 3.0, 12.0, true},
	{"anthropic/claude-3.5-sonnet", "anthropic", 3.0, 15.0, true},
	{"anthropic/claude-3-opus", "anthropic", 15.0, 75.0, false},
	{"anthropic/claude-3-haiku", "anthropic", 0.25, 1.25, false},
	{"anthropic/claude-4.5-opus", "anthropic", 5.0, 25.0, true},
	{"anthropic/claude-opus-4.6", "anthropic", 5.0, 25.0, true},
	{"anthropic/claude-4.5-sonnet", "anthropic", 1.0, 15.0, true},
	{"z-ai/glm-4.7", "z-ai", 0.4, 1.5, true},
	{"deepseek/deepseek-chat", "deepseek", 0.14, 0.28, false},
	{"deepseek/deepseek-r1", "deepseek", 0.55, 2.19, true},
	{"openrouter/free", "openrouter", 0, 0, true},
	{"moonshotai/kimi-k2-thinking", "moonshotai", 0.4, 1.75, true},
	{"moonshotai/kimi-k2.5", "moonshotai", 0.45, 2.50, true},
}

func SupportedModels() []string {
	out := make([]string, 0, len(modelInfos))
	for _, info := range modelInfos {
		id := strings.TrimSpace(info.ID)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func SupportedModelInfos() []ModelInfo {
	out := make([]ModelInfo, 0, len(modelInfos))
	for _, info := range modelInfos {
		if strings.TrimSpace(info.ID) == "" {
			continue
		}
		copy := info
		if strings.TrimSpace(copy.Provider) == "" {
			if idx := strings.Index(copy.ID, "/"); idx > 0 {
				copy.Provider = copy.ID[:idx]
			}
		}
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func SupportsReasoningEffort(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return false
	}
	for _, info := range modelInfos {
		if strings.EqualFold(strings.TrimSpace(info.ID), id) {
			return info.IsReasoning
		}
	}
	for _, info := range modelInfos {
		key := strings.TrimSpace(info.ID)
		if key == "" {
			continue
		}
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.ToLower(strings.TrimSpace(key[idx+1:]))
			if suffix == id {
				return info.IsReasoning
			}
		}
	}
	return false
}

func SupportsReasoningSummary(modelID string) bool {
	return SupportsReasoningEffort(modelID)
}
