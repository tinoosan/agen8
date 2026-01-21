package cost

import (
	"sort"
	"strings"
)

// ModelInfo describes a recognition+pricing entry for a model provider.
// InputPerM/OutputPerM remain 0 when pricing is unknown.
type ModelInfo struct {
	ID          string
	InputPerM   float64
	OutputPerM  float64
	IsReasoning bool
}

var modelInfos = []ModelInfo{
	// OpenAI (via OpenRouter).
	{"openai/gpt-5.2", 1.75, 14.0, true},
	{"openai/gpt-5.2-chat", 1.75, 14.0, true},
	{"openai/gpt-5.2-pro", 21.0, 168.0, true},
	{"openai/gpt-5.1", 1.25, 10.0, true},
	{"openai/gpt-5.1-chat", 1.25, 10.0, true},
	{"openai/gpt-5.1-codex", 1.25, 10.0, true},
	{"openai/gpt-5.1-codex-mini", 0.25, 2.0, true},
	{"openai/gpt-5.1-codex-max", 1.25, 10.0, true},
	{"openai/gpt-5-mini", 0.25, 2.0, true},
	{"openai/gpt-5-nano", 0.05, 0.4, true},
	{"openai/gpt-4.1", 0, 0, false}, // Pricing to be confirmed.
	{"openai/gpt-4o", 2.5, 10.0, false},
	{"openai/gpt-4o-mini", 0.15, 0.6, false},
	{"openai/o1-preview", 15.0, 60.0, true},
	{"openai/o1-mini", 3.0, 12.0, true},

	// Anthropic.
	{"anthropic/claude-3.5-sonnet", 3.0, 15.0, true},
	{"anthropic/claude-3-opus", 15.0, 75.0, false},
	{"anthropic/claude-3-haiku", 0.25, 1.25, false},
	{"anthropic/claude-4.5-opus", 5.0, 25.0, true},
	{"anthropic/claude-4.5-sonnet", 1.0, 15.0, true},

	// Z.AI.
	{"z-ai/glm-4.7", 0.4, 1.5, true},

	// DeepSeek.
	{"deepseek/deepseek-chat", 0.14, 0.28, false},
	{"deepseek/deepseek-r1", 0.55, 2.19, true},
}

// SupportedModels returns the list of model IDs Workbench recognizes.
//
// This list is used by the interactive /model picker to validate user input.
//
// Pricing remains separate: models with zero values are treated as "unknown".
// To add a model, append it to `modelInfos` and, if you know pricing, set
// InputPerM/OutputPerM accordingly.
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

// IsSupportedModel returns true if id is in the supported model list.
func IsSupportedModel(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, info := range modelInfos {
		if strings.TrimSpace(info.ID) == id {
			return true
		}
	}
	return false
}

// SupportsReasoningEffort reports whether the model is expected to support explicit
// reasoning/thinking controls (best-effort; provider/model dependent).
//
// Keep this close to the supported model list so adding a model is a single-file change.
// Conservative default: return false unless we explicitly recognize the model family.
func SupportsReasoningEffort(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return false
	}
	// Look up in our model registry
	for _, info := range modelInfos {
		if strings.EqualFold(info.ID, id) {
			return info.IsReasoning
		}
	}
	return false
}

// SupportsReasoningSummary reports whether the model is expected to support
// provider-supplied reasoning summaries (safe-to-display) via OpenRouter's
// OpenAI-compatible Responses/Chat APIs.
//
// For now we treat this capability as equivalent to SupportsReasoningEffort.
func SupportsReasoningSummary(modelID string) bool {
	return SupportsReasoningEffort(modelID)
}
