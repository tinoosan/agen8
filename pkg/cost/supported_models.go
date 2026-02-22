package cost

import (
	"sort"
	"strings"
)

type ModelInfo struct {
	ID            string
	Provider      string
	InputPerM     float64
	OutputPerM    float64
	IsReasoning   bool
	ContextLength int
}

var modelInfos = []ModelInfo{
	{"openai/gpt-5.2", "openai", 1.75, 14.0, true, 128000},
	{"openai/gpt-5.2-chat", "openai", 1.75, 14.0, true, 128000},
	{"openai/gpt-5.2-pro", "openai", 21.0, 168.0, true, 128000},
	{"openai/gpt-5.1", "openai", 1.25, 10.0, true, 128000},
	{"openai/gpt-5.1-chat", "openai", 1.25, 10.0, true, 128000},
	{"openai/gpt-5.1-codex", "openai", 1.25, 10.0, true, 128000},
	{"openai/gpt-5.1-codex-mini", "openai", 0.25, 2.0, true, 128000},
	{"openai/gpt-5.1-codex-max", "openai", 1.25, 10.0, true, 128000},
	{"openai/gpt-5-mini", "openai", 0.25, 2.0, true, 128000},
	{"openai/gpt-5-nano", "openai", 0.05, 0.4, true, 128000},
	{"openai/gpt-4.1", "openai", 2, 8, false, 128000},
	{"openai/gpt-4o", "openai", 2.5, 10.0, false, 128000},
	{"openai/gpt-4o-mini", "openai", 0.15, 0.6, false, 128000},
	{"openai/o1-preview", "openai", 15.0, 60.0, true, 128000},
	{"openai/o1-mini", "openai", 3.0, 12.0, true, 128000},
	{"openai/gpt-oss-20b:free", "openai", 0, 0, false, 8192},
	{"openai/gpt-oss-120b:free", "openai", 0, 0, false, 8192},
	{"anthropic/claude-3.5-sonnet", "anthropic", 3.0, 15.0, true, 200000},
	{"anthropic/claude-3-opus", "anthropic", 15.0, 75.0, false, 200000},
	{"anthropic/claude-3-haiku", "anthropic", 0.25, 1.25, false, 200000},
	{"anthropic/claude-4.5-opus", "anthropic", 5.0, 25.0, true, 200000},
	{"anthropic/claude-opus-4.6", "anthropic", 5.0, 25.0, true, 200000},
	{"anthropic/claude-4.5-sonnet", "anthropic", 3.0, 15.0, true, 200000},
	{"anthropic/claude-4.6-sonnet", "anthropic", 3.0, 15.0, true, 200000},
	{"z-ai/glm-4.5-air:free", "z-ai", 0, 0, false, 128000},
	{"z-ai/glm-4.7", "z-ai", 0.4, 1.5, true, 128000},
	{"z-ai/glm-4.7-flash", "z-ai", 0.06, 0.40, true, 128000},
	{"z-ai/glm-5", "z-ai", 0.8, 2.56, true, 128000},
	{"deepseek/deepseek-chat", "deepseek", 0.14, 0.28, false, 64000},
	{"deepseek/deepseek-r1", "deepseek", 0.55, 2.19, true, 64000},
	{"openrouter/free", "openrouter", 0, 0, true, 8192},
	{"moonshotai/kimi-k2-thinking", "moonshotai", 0.4, 1.75, true, 262000},
	{"moonshotai/kimi-k2.5", "moonshotai", 0.45, 2.50, false, 262000},
	{"minimax/minimax-m2.5", "minimax", 0.30, 1.10, true, 24576},
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
	id := normalizeModelID(modelID)
	if id == "" {
		return false
	}
	for _, info := range modelInfos {
		if normalizeModelID(info.ID) == id {
			return info.IsReasoning
		}
	}
	for _, info := range modelInfos {
		key := normalizeModelID(info.ID)
		if key == "" {
			continue
		}
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.TrimSpace(key[idx+1:])
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

func normalizeModelID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ""
	}
	if i := strings.Index(id, "?"); i >= 0 {
		id = strings.TrimSpace(id[:i])
	}
	if i := strings.Index(id, "#"); i >= 0 {
		id = strings.TrimSpace(id[:i])
	}
	if slash := strings.LastIndex(id, "/"); slash >= 0 && slash+1 < len(id) {
		head := id[:slash+1]
		tail := id[slash+1:]
		if colon := strings.Index(tail, ":"); colon > 0 {
			tail = tail[:colon]
		}
		id = head + tail
	} else if colon := strings.Index(id, ":"); colon > 0 {
		id = id[:colon]
	}
	return strings.TrimSpace(id)
}

func ContextLengthForModel(modelID string) (int, bool) {
	id := normalizeModelID(modelID)
	if id == "" {
		return 0, false
	}
	for _, info := range modelInfos {
		if normalizeModelID(info.ID) == id {
			return info.ContextLength, info.ContextLength > 0
		}
	}

	idSuffix := id
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx+1 < len(id) {
		idSuffix = id[idx+1:]
	}

	for _, info := range modelInfos {
		key := normalizeModelID(info.ID)
		if key == "" {
			continue
		}
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.TrimSpace(key[idx+1:])
			if suffix == id || suffix == idSuffix {
				return info.ContextLength, info.ContextLength > 0
			}
		}
	}
	return 0, false
}
