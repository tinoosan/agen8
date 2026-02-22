package cost

import (
	"context"
	"os"
	"testing"
)

func TestContextBudgetTokens(t *testing.T) {
	ctx := context.Background()

	// 1. Env override
	os.Setenv("AGEN8_CONTEXT_BUDGET_TOKENS", "50000")
	t.Cleanup(func() { os.Unsetenv("AGEN8_CONTEXT_BUDGET_TOKENS") })

	budget := ContextBudgetTokens(ctx, "openai/gpt-5.2")
	if budget != 50000 {
		t.Errorf("Expected 50000 from env, got %d", budget)
	}

	// 2. Registry value
	os.Unsetenv("AGEN8_CONTEXT_BUDGET_TOKENS")
	budget = ContextBudgetTokens(ctx, "openai/gpt-5.2")
	if budget != 128000 {
		t.Errorf("Expected 128000 from registry, got %d", budget)
	}

	// 3. Fallback default
	budget = ContextBudgetTokens(ctx, "unknown/model")
	if budget != 128000 {
		t.Errorf("Expected default 128000 for unknown model, got %d", budget)
	}
}
