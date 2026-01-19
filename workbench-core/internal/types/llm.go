package types

import (
	"context"
	"encoding/json"
)

// LLMClient is the minimal interface used by the agent loop to request a model completion.
//
// v0 scope:
//   - text in, text out
//   - no streaming
//   - no tool/function calling (the agent loop is responsible for executing HostOpRequest)
//
// The agent loop uses LLMClient.Generate to ask "what should I do next?" and expects
// either a HostOpRequest JSON object or a terminal {"op":"final","text":"..."} JSON object.
type LLMClient interface {
	Generate(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

// LLMStreamChunk is a provider-agnostic streaming output unit.
//
// Phase 1 scope:
//   - Text: incremental assistant content as it arrives
//   - Done: optional sentinel (providers may also use EOF/[DONE])
type LLMStreamChunk struct {
	Text        string
	IsReasoning bool // true if this chunk is reasoning/thinking content (provider-specific)
	Done        bool
}

// LLMStreamCallback is invoked for each stream chunk.
//
// Returning a non-nil error aborts streaming and should cancel the request.
type LLMStreamCallback func(chunk LLMStreamChunk) error

// LLMClientStreaming is an optional extension interface for providers that support
// token streaming.
//
// GenerateStream should:
//   - invoke cb as chunks arrive
//   - return the final accumulated response text (and optional usage/raw)
type LLMClientStreaming interface {
	GenerateStream(ctx context.Context, req LLMRequest, cb LLMStreamCallback) (LLMResponse, error)
}

// LLMRequest is a provider-agnostic request shape for a text completion.
//
// System is the system prompt string (developer instructions). Messages represent the
// conversational transcript (user/assistant turns).
type LLMRequest struct {
	Model              string       // required: model identifier (provider-specific)
	System             string       // optional: system prompt
	Messages           []LLMMessage // required: conversation messages
	MaxTokens          int          // optional: max output tokens
	Temperature        float64      // optional: sampling temperature
	JSONOnly           bool         // optional: request JSON-only output (provider best-effort)
	PreviousResponseID string       // optional: for Responses API reasoning context
}

// LLMMessage is a minimal chat message.
type LLMMessage struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// LLMResponse is the minimal response used by the agent loop.
type LLMResponse struct {
	Text       string          // assistant content (raw text)
	Raw        json.RawMessage // optional: raw provider JSON response (debug)
	Usage      *LLMUsage       // optional: token usage
	ResponseID string          // optional: response ID for Responses API
}

// LLMUsage contains token usage numbers when a provider returns them.
type LLMUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
