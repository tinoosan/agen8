package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/llm"
)

// Integration test: requires OPENROUTER_API_KEY and RUN_LLM_INTEGRATION=1.
func TestGenerateSessionTitle_Integration(t *testing.T) {
	if os.Getenv("RUN_LLM_INTEGRATION") != "1" {
		t.Skip("set RUN_LLM_INTEGRATION=1 to run integration test")
	}
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("OPENROUTER_API_KEY is required for integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	title, err := generateSessionTitle(ctx, "Build a small CLI tool to batch rename photos by date.")
	if err != nil {
		t.Fatalf("generateSessionTitle error: %v", err)
	}
	if title == "" {
		cli, err := llm.NewClientFromEnv()
		if err != nil {
			t.Fatalf("expected non-empty title (and llm client failed: %v)", err)
		}
		req := llm.LLMRequest{
			Model:       sessionTitleModel,
			System:      "Create a short, descriptive session title. Use 3-8 words. Return only the title.",
			Messages:    []llm.LLMMessage{{Role: "user", Content: "Build a small CLI tool to batch rename photos by date."}},
			MaxTokens:   sessionTitleMaxTokens,
			Temperature: 0.2,
		}
		resp, err := cli.Generate(ctx, req)
		if err != nil {
			t.Fatalf("expected non-empty title (llm error: %v)", err)
		}
		t.Fatalf("expected non-empty title (raw=%s text=%q)", string(resp.Raw), resp.Text)
	}
}
