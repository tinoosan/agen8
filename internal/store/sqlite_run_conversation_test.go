package store

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
)

func TestSQLiteRunConversationStore(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		DataDir: t.TempDir(),
	}

	store, err := NewSQLiteRunConversationStoreFromConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	runID := "test-run-1"

	// 1. Load empty state
	msgs, err := store.LoadMessages(ctx, runID)
	if err != nil {
		t.Fatalf("LoadMessages expected nil error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("LoadMessages expected empty, got %d", len(msgs))
	}

	// 2. Save messages
	toSave := []llmtypes.LLMMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}
	err = store.SaveMessages(ctx, runID, toSave)
	if err != nil {
		t.Fatalf("SaveMessages failed: %v", err)
	}

	// 3. Load populated state
	msgs, err = store.LoadMessages(ctx, runID)
	if err != nil {
		t.Fatalf("LoadMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "Hello" || msgs[1].Content != "Hi there" {
		t.Errorf("Unexpected messages: %+v", msgs)
	}

	// 4. Overwrite messages (appending)
	toSave = append(toSave, llmtypes.LLMMessage{Role: "user", Content: "Next interaction"})
	err = store.SaveMessages(ctx, runID, toSave)
	if err != nil {
		t.Fatalf("SaveMessages overwrite failed: %v", err)
	}

	// 5. Load overwritten state
	msgs, err = store.LoadMessages(ctx, runID)
	if err != nil {
		t.Fatalf("LoadMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}
	if msgs[2].Content != "Next interaction" {
		t.Errorf("Unexpected message 3: %+v", msgs[2])
	}
}
