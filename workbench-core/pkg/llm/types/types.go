package types

import (
	"context"
	"encoding/json"
)

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
	Strict      bool        `json:"strict,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type LLMClient interface {
	Generate(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

type LLMStreamChunk struct {
	Text        string
	IsReasoning bool
	Done        bool
}

type LLMStreamCallback func(chunk LLMStreamChunk) error

type LLMClientStreaming interface {
	GenerateStream(ctx context.Context, req LLMRequest, cb LLMStreamCallback) (LLMResponse, error)
}

type LLMRequest struct {
	Model              string
	System             string
	Messages           []LLMMessage
	MaxTokens          int
	Temperature        float64
	JSONOnly           bool
	Tools              []Tool `json:"tools,omitempty"`
	ToolChoice         string `json:"toolChoice,omitempty"`
	EnableWebSearch    bool   `json:"enableWebSearch,omitempty"`
	ResponseSchema     *LLMResponseSchema
	PreviousResponseID string
	ReasoningEffort    string
	ReasoningSummary   string
}

type LLMResponseSchema struct {
	Name        string
	Schema      map[string]any
	Strict      bool
	Description string
}

type LLMMessage struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type LLMResponse struct {
	Text       string          // assistant content (raw text)
	Raw        json.RawMessage // optional: raw provider JSON response (debug)
	Usage      *LLMUsage       // optional: token usage
	ResponseID string          // optional: response ID for Responses API
	ToolCalls  []ToolCall      `json:"toolCalls,omitempty"`
	Citations  []LLMCitation   `json:"citations,omitempty"`
}

type LLMCitation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type LLMUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
