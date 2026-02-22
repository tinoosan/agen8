package store

import (
	"context"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
)

// RunConversationStore persists the conversational context of a run across tasks.
type RunConversationStore interface {
	LoadMessages(ctx context.Context, runID string) ([]llmtypes.LLMMessage, error)
	SaveMessages(ctx context.Context, runID string, msgs []llmtypes.LLMMessage) error
}
