package cost

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// ContextBudgetTokens returns the context budget in tokens for a given model.
// It checks the environment variable, the model registry, and finally OpenRouter.
func ContextBudgetTokens(ctx context.Context, modelID string) int {
	if v := strings.TrimSpace(os.Getenv("AGEN8_CONTEXT_BUDGET_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}

	if n, ok := ContextLengthFromOpenRouter(ctx, modelID); ok && n > 0 {
		return n
	}

	if n, ok := ContextLengthForModel(modelID); ok && n > 0 {
		return n
	}

	// Safe default
	return 128000
}
