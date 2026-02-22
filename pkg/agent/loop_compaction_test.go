package agent

import (
	"context"
	"strings"
	"testing"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
)

type fakeCompactingLLM struct {
	supported bool
	out       []llmtypes.LLMMessage
	err       error
}

func (f *fakeCompactingLLM) Generate(context.Context, llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	return llmtypes.LLMResponse{}, nil
}

func (f *fakeCompactingLLM) SupportsStreaming() bool { return false }

func (f *fakeCompactingLLM) SupportsServerCompaction() bool { return f.supported }

func (f *fakeCompactingLLM) CompactConversation(context.Context, llmtypes.LLMCompactionRequest) (llmtypes.LLMCompactionResponse, error) {
	return llmtypes.LLMCompactionResponse{Messages: f.out}, f.err
}

func TestDefaultAgent_CompactConversationForBudget_UsesServerCompaction(t *testing.T) {
	llm := &fakeCompactingLLM{
		supported: true,
		out:       []llmtypes.LLMMessage{{Role: "user", Content: "goal"}, {Role: "compaction", Content: "enc"}},
	}
	a := &DefaultAgent{LLM: llm, Model: "gpt-5"}
	msgs := []llmtypes.LLMMessage{
		{Role: "user", Content: strings.Repeat("u", 4096)},
		{Role: "assistant", Content: strings.Repeat("a", 4096)},
	}
	got := a.compactConversationForBudget(context.Background(), 1, msgs, "system", 128)
	if len(got) != 3 {
		t.Fatalf("len(got)=%d, want 3 (developer notice + 2 compacted)", len(got))
	}
	if got[0].Role != "developer" || !strings.Contains(got[0].Content, "server-side") {
		t.Fatalf("expected server-side compaction notice as first message, got %+v", got[0])
	}
	if got[2].Role != "compaction" || got[2].Content != "enc" {
		t.Fatalf("unexpected compacted output: %+v", got)
	}
}

func TestDefaultAgent_CompactConversationForBudget_FallsBackToLocalCompaction(t *testing.T) {
	llm := &fakeCompactingLLM{
		supported: true,
		err:       context.DeadlineExceeded,
	}
	a := &DefaultAgent{LLM: llm, Model: "gpt-5"}
	msgs := make([]llmtypes.LLMMessage, 0, 64)
	for i := 0; i < 64; i++ {
		msgs = append(msgs, llmtypes.LLMMessage{
			Role:    "user",
			Content: strings.Repeat("x", 300),
		})
	}
	got := a.compactConversationForBudget(context.Background(), 1, msgs, "system", 1024)
	foundNotice := false
	for _, m := range got {
		if m.Role == "developer" && strings.Contains(m.Content, "Context was compacted automatically") {
			foundNotice = true
			break
		}
	}
	if !foundNotice {
		t.Fatalf("expected local compaction notice in output, got %+v", got)
	}
}
