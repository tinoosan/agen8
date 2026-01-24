package types

import (
	"context"
	"encoding/json"
)

// Tool represents a function/tool the model can call.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"` // JSON schema
	Strict      bool        `json:"strict,omitempty"`
}

// ToolCall represents a tool/function call from the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

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
	Model       string       // required: model identifier (provider-specific)
	System      string       // optional: system prompt
	Messages    []LLMMessage // required: conversation messages
	MaxTokens   int          // optional: max output tokens
	Temperature float64      // optional: sampling temperature
	JSONOnly    bool         // optional: request JSON-only output (provider best-effort)
	Tools       []Tool       `json:"tools,omitempty"`
	ToolChoice  string       `json:"toolChoice,omitempty"` // "auto", "none", "required", or "function:<toolName>"
	// EnableWebSearch requests real-time web search grounding when supported by the provider.
	// For OpenRouter this is implemented by using a model variant like ":online".
	EnableWebSearch bool `json:"enableWebSearch,omitempty"`
	// ResponseSchema optionally requests Structured Outputs. When set, providers that
	// support it should constrain output to exactly match the schema.
	//
	// Notes:
	// - Only a subset of JSON Schema is supported in strict mode (provider-specific).
	// - When ResponseSchema is set, clients should prefer json_schema over json_object.
	ResponseSchema     *LLMResponseSchema
	PreviousResponseID string // optional: for Responses API reasoning context
	ReasoningEffort    string // optional: none|minimal|low|medium|high|xhigh (provider best-effort)
	ReasoningSummary   string // optional: off|auto|concise|detailed (provider best-effort)
}

// LLMResponseSchema describes a Structured Outputs schema request.
//
// Schema should be a JSON Schema object represented as a Go value suitable for
// JSON serialization (commonly `map[string]any`).
type LLMResponseSchema struct {
	// Name is a short identifier for the schema (provider constraints apply).
	Name string
	// Schema is the JSON Schema object for the response format.
	Schema map[string]any
	// Strict requests strict schema adherence when supported.
	Strict bool
	// Description is optional provider-facing guidance for the schema.
	Description string
}

// LLMMessage is a minimal chat message.
type LLMMessage struct {
	Role       string // "system" | "user" | "assistant" | "tool"
	Content    string
	ToolCallID string // used when Role=="tool": tool_call_id
	ToolCalls  []ToolCall
}

// LLMResponse is the minimal response used by the agent loop.
type LLMResponse struct {
	Text       string          // assistant content (raw text)
	Raw        json.RawMessage // optional: raw provider JSON response (debug)
	Usage      *LLMUsage       // optional: token usage
	ResponseID string          // optional: response ID for Responses API
	ToolCalls  []ToolCall      `json:"toolCalls,omitempty"`
	Citations  []LLMCitation   `json:"citations,omitempty"`
}

// LLMCitation is a URL citation associated with generated text.
// Providers typically supply these via "url_citation" annotations.
type LLMCitation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// LLMUsage contains token usage numbers when a provider returns them.
type LLMUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
