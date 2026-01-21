package cost

import (
	"sort"
	"strings"
)

// ModelInfo describes a recognition+pricing entry for a model provider.
// InputPerM/OutputPerM remain 0 when pricing is unknown.
type ModelInfo struct {
	ID         string
	InputPerM  float64
	OutputPerM float64
}

var modelInfos = []ModelInfo{
	// OpenAI (via OpenRouter).
	{"openai/gpt-5.2", 1.75, 14.0},
	{"openai/gpt-5.2-chat", 1.75, 14.0},
	{"openai/gpt-5.2-pro", 21.0, 168.0},
	{"openai/gpt-5.1", 1.25, 10.0},
	{"openai/gpt-5.1-chat", 1.25, 10.0},
	{"openai/gpt-5.1-codex", 1.25, 10.0},
	{"openai/gpt-5.1-codex-mini", 0.25, 2.0},
	{"openai/gpt-5.1-codex-max", 1.25, 10.0},
	{"openai/gpt-5-mini", 0.25, 2.0},
	{"openai/gpt-5-nano", 0.05, 0.4},
	{"openai/gpt-4.1", 0, 0}, // Pricing to be confirmed.
	{"openai/gpt-4o", 2.5, 10.0},
	{"openai/gpt-4o-mini", 0.15, 0.6},
	{"openai/o1-preview", 15.0, 60.0},
	{"openai/o1-mini", 3.0, 12.0},

	// Anthropic.
	{"anthropic/claude-3.5-sonnet", 3.0, 15.0},
	{"anthropic/claude-3-opus", 15.0, 75.0},
	{"anthropic/claude-3-haiku", 0.25, 1.25},
	{"anthropic/claude-4.5-opus", 5.0, 25.0},
	{"anthropic/claude-4.5-sonnet", 1.0, 15.0},

	// Google.
	{"google/gemini-pro-1.5", 2.5, 7.5},
	{"google/gemini-flash-1.5", 0.075, 0.3},

	// Meta.
	{"meta-llama/llama-3.1-405b-instruct", 2.7, 2.7},
	{"meta-llama/llama-3.1-70b-instruct", 0.35, 0.4},
	{"meta-llama/llama-3.2-11b-vision-instruct", 0.055, 0.055},
	{"meta-llama/llama-3.2-3b-instruct", 0.04, 0.04},

	// Mistral.
	{"mistralai/mistral-large", 2.0, 6.0},

	// Z.AI.
	{"z-ai/glm-4.7", 0.4, 1.5},

	// DeepSeek.
	{"deepseek/deepseek-chat", 0.14, 0.28},
	{"deepseek/deepseek-r1", 0.55, 2.19},

	// Qwen.
	{"qwen/qwen-2.5-72b-instruct", 0.35, 0.4},
	{"qwen/qwen-2.5-coder-32b-instruct", 0.07, 0.16},
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

	// OpenAI GPT-5 family (reasoning controls).
	// Examples: openai/gpt-5.2, openai/gpt-5.2-pro, openai/gpt-5.2-chat, openai/gpt-5.1-*
	if strings.HasPrefix(id, "openai/gpt-5") {
		return true
	}

	// OpenAI o-series reasoning models.
	// Examples: openai/o4-mini, openai/o3, openai/o3-mini, openai/o1, openai/o1-mini
	if strings.HasPrefix(id, "openai/o1") || strings.HasPrefix(id, "openai/o3") || strings.HasPrefix(id, "openai/o4") {
		return true
	}

	// OpenAI open-weights reasoning-control line (when routed via OpenRouter).
	// Example: openai/gpt-oss-120b
	if strings.Contains(id, "gpt-oss") {
		return true
	}

	// Anthropic "extended thinking" models (explicit thinking mode).
	// Examples from vendor docs: claude-opus-4-*, claude-sonnet-4-*, claude-3-7-sonnet-*
	if strings.HasPrefix(id, "anthropic/claude-opus-4") ||
		strings.HasPrefix(id, "anthropic/claude-sonnet-4") ||
		strings.HasPrefix(id, "anthropic/claude-4") ||
		strings.HasPrefix(id, "anthropic/claude-4.5-opus-") ||
		strings.HasPrefix(id, "anthropic/claude-4.5-sonnet") {
		return true
	}
	if strings.Contains(id, "claude-3.7-sonnet") || strings.Contains(id, "claude-3-7-sonnet") {
		return true
	}

	// Google Gemini "thinking models".
	// Examples from vendor docs: gemini-2.5-pro, gemini-2.5-flash
	if strings.HasPrefix(id, "google/gemini-2.5") {
		return true
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
