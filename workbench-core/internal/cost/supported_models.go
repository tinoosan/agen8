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
	"openai/gpt-4.1",
	"openai/gpt-4o",
	"openai/gpt-4o-mini",

	// O-series (example placeholders; add only if you actually use them).
	// "openai/o3",
	// "openai/o3-mini",
}
