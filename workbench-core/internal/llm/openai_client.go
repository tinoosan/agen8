package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
	"github.com/tinoosan/workbench-core/internal/types"
)

// Client implements types.LLMClient and types.LLMClientStreaming using the official
// OpenAI Go SDK, pointed at OpenRouter's OpenAI-compatible endpoint.
type Client struct {
	client *openai.Client

	// DefaultMaxTokens is used when LLMRequest.MaxTokens is 0.
	DefaultMaxTokens int
}

func NewClientFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	defaultMaxTokens := 1024
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_MAX_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			defaultMaxTokens = n
		}
	}

	cli := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(strings.TrimRight(baseURL, "/")),
	)

	return &Client{
		client:           &cli,
		DefaultMaxTokens: defaultMaxTokens,
	}, nil
}

func (c *Client) buildParams(req types.LLMRequest) (openai.ChatCompletionNewParams, error) {
	if c == nil || c.client == nil {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("llm client is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("model is required")
	}

	// Message mapping: prepend explicit system prompt as a system message.
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			msgs = append(msgs, openai.SystemMessage(m.Content))
		case "assistant":
			msgs = append(msgs, openai.AssistantMessage(m.Content))
		case "developer":
			msgs = append(msgs, openai.DeveloperMessage(m.Content))
		default:
			// Treat unknown roles as user.
			msgs = append(msgs, openai.UserMessage(m.Content))
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.DefaultMaxTokens
	}
	if maxTokens < 0 {
		return openai.ChatCompletionNewParams{}, fmt.Errorf("maxTokens must be >= 0")
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(req.Model),
		Messages: msgs,
	}
	if maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.JSONOnly {
		rf := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &rf,
		}
	}
	return params, nil
}

func (c *Client) toResponse(resp *openai.ChatCompletion) (types.LLMResponse, error) {
	if resp == nil {
		return types.LLMResponse{}, fmt.Errorf("response is nil")
	}

	text := ""
	if len(resp.Choices) != 0 {
		text = strings.TrimSpace(resp.Choices[0].Message.Content)
	}

	out := types.LLMResponse{Text: text}

	if raw := strings.TrimSpace(resp.RawJSON()); raw != "" {
		out.Raw = json.RawMessage(raw)
	}

	// If usage was not provided, these will be 0.
	if resp.Usage.TotalTokens != 0 || resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		}
	}

	return out, nil
}

func (c *Client) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}
	params, err := c.buildParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}
	resp, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return types.LLMResponse{}, err
	}
	return c.toResponse(resp)
}

func (c *Client) onStreamChunk(acc *openai.ChatCompletionAccumulator, chunk openai.ChatCompletionChunk, cb types.LLMStreamCallback) error {
	if acc != nil {
		_ = acc.AddChunk(chunk)
	}
	if cb == nil || len(chunk.Choices) == 0 {
		return nil
	}

	delta := chunk.Choices[0].Delta

	// Some OpenAI-compatible providers (including OpenRouter-backed reasoning models)
	// return reasoning fields that aren't part of the standard SDK struct.
	//
	// IMPORTANT: we do not forward raw reasoning text. We only:
	// - emit a reasoning signal (IsReasoning=true, Text="")
	// - forward an explicit reasoning summary if provided separately
	// Summary (safe to show if the provider returns it explicitly).
	if s, ok := deltaString(delta, "reasoning_summary"); ok {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}
	if s, ok := deltaString(delta, "reasoning_summary_text"); ok {
		if err := cb(types.LLMStreamChunk{Text: s, IsReasoning: true}); err != nil {
			return err
		}
	}

	// Raw reasoning (do not display).
	if s, ok := deltaString(delta, "reasoning_content"); ok && strings.TrimSpace(s) != "" {
		if err := cb(types.LLMStreamChunk{IsReasoning: true}); err != nil {
			return err
		}
	}
	if s, ok := deltaString(delta, "reasoning"); ok && strings.TrimSpace(s) != "" {
		if err := cb(types.LLMStreamChunk{IsReasoning: true}); err != nil {
			return err
		}
	}

	// Standard streamed content.
	// Important: do NOT TrimSpace here. Providers can stream single-space deltas,
	// and the agent's JSON-string decoder relies on receiving them.
	if delta.Content != "" {
		return cb(types.LLMStreamChunk{Text: delta.Content, IsReasoning: false})
	}
	return nil
}

func (c *Client) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}
	params, err := c.buildParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	if stream == nil {
		return types.LLMResponse{}, fmt.Errorf("stream is nil")
	}

	var acc openai.ChatCompletionAccumulator

	for stream.Next() {
		chunk := stream.Current()
		if err := c.onStreamChunk(&acc, chunk, cb); err != nil {
			return types.LLMResponse{}, err
		}
	}
	if err := stream.Err(); err != nil {
		return types.LLMResponse{}, err
	}

	if cb != nil {
		if err := cb(types.LLMStreamChunk{Done: true}); err != nil {
			return types.LLMResponse{}, err
		}
	}

	return c.toResponse(&acc.ChatCompletion)
}

func extraString(fields map[string]respjson.Field, key string) (string, bool) {
	if fields == nil || strings.TrimSpace(key) == "" {
		return "", false
	}
	f, ok := fields[key]
	if !ok || !f.Valid() {
		return "", false
	}
	raw := strings.TrimSpace(f.Raw())
	if raw == "" || raw == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}

func deltaString(delta openai.ChatCompletionChunkChoiceDelta, key string) (string, bool) {
	if strings.TrimSpace(key) == "" {
		return "", false
	}

	// Preferred: parsed extra fields (when available).
	if fields := delta.JSON.ExtraFields; fields != nil {
		if s, ok := extraString(fields, key); ok {
			return s, true
		}
	}

	// Fallback: parse raw delta JSON (some providers may not populate ExtraFields).
	raw := strings.TrimSpace(delta.RawJSON())
	if raw == "" {
		return "", false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false
	}
	b, ok := m[key]
	if !ok || len(b) == 0 || strings.TrimSpace(string(b)) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return "", false
	}
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	return s, true
}
