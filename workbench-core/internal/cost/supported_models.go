package cost

import (
	"sort"
	"strings"
)

// SupportedModels returns the list of model IDs Workbench recognizes.
//
// This list is used by the interactive /model picker to validate user input.
//
// Pricing is deliberately separate:
//   - A supported model may have no pricing entry.
//   - When pricing is missing, Workbench shows cost as "unknown" (and does not
//     fabricate a number).
//
// To add a model:
//  1. Add it to `supportedModelIDs`.
//  2. Optionally add pricing in DefaultPricing().
func SupportedModels() []string {
	out := make([]string, 0, len(supportedModelIDs))
	for _, id := range supportedModelIDs {
		id = strings.TrimSpace(id)
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
	for _, m := range supportedModelIDs {
		if strings.TrimSpace(m) == id {
			return true
		}
	}
	return false
}

var supportedModelIDs = []string{
	// OpenAI (via OpenRouter).
	"openai/gpt-5.2",
	"openai/gpt-5.2-chat",
	"openai/gpt-5.2-pro",
	"openai/gpt-5.1",
	"openai/gpt-5.1-chat",
	"openai/gpt-5.1-codex",
	"openai/gpt-5.1-codex-mini",
	"openai/gpt-5.1-codex-max",
	"openai/gpt-5-mini",
	"openai/gpt-5-nano",
	"openai/gpt-4.1",
	"openai/gpt-4o",
	"openai/gpt-4o-mini",
	"openai/o1-preview",
	"openai/o1-mini",

	// Anthropic.
	"anthropic/claude-3.5-sonnet",
	"anthropic/claude-3-opus",
	"anthropic/claude-3-haiku",
	"anthropic/claude-4.5-opus",
	"anthropic/claude-4.5-sonnet",

	// Google.
	"google/gemini-pro-1.5",
	"google/gemini-flash-1.5",

	// Meta.
	"meta-llama/llama-3.1-405b-instruct",
	"meta-llama/llama-3.1-70b-instruct",
	"meta-llama/llama-3.2-11b-vision-instruct",
	"meta-llama/llama-3.2-3b-instruct",

	// Mistral.
	"mistralai/mistral-large",

	// Z.AI.
	"z-ai/glm-4.7",

	// DeepSeek.
	"deepseek/deepseek-chat",
	"deepseek/deepseek-r1",

	// Qwen.
	"qwen/qwen-2.5-72b-instruct",
	"qwen/qwen-2.5-coder-32b-instruct",

	// O-series (example placeholders; add only if you actually use them).
	// "openai/o3",
	// "openai/o3-mini",
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
