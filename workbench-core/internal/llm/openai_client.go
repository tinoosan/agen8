package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/responses"
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

func (c *Client) buildResponseParams(req types.LLMRequest) (responses.ResponseNewParams, error) {
	if c == nil || c.client == nil {
		return responses.ResponseNewParams{}, fmt.Errorf("llm client is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return responses.ResponseNewParams{}, fmt.Errorf("model is required")
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.DefaultMaxTokens
	}
	if maxTokens < 0 {
		return responses.ResponseNewParams{}, fmt.Errorf("maxTokens must be >= 0")
	}

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)

	// Message mapping: Responses API uses an input item list with explicit roles.
	// System/developer instructions are provided via `Instructions`.
	msgs := req.Messages
	// When chaining with previous_response_id, send only the newest message (delta-only)
	// to avoid duplicating the transcript in the model context.
	if previousResponseID != "" && len(msgs) > 0 {
		msgs = msgs[len(msgs)-1:]
	}

	items := make(responses.ResponseInputParam, 0, len(msgs))
	for _, m := range msgs {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		rrole := responses.EasyInputMessageRoleUser
		switch role {
		case "system":
			rrole = responses.EasyInputMessageRoleSystem
		case "developer":
			rrole = responses.EasyInputMessageRoleDeveloper
		case "assistant":
			rrole = responses.EasyInputMessageRoleAssistant
		default:
			rrole = responses.EasyInputMessageRoleUser
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, rrole))
	}

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(req.Model),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
		},
	}
	if previousResponseID != "" {
		params.PreviousResponseID = openai.String(previousResponseID)
	}
	if strings.TrimSpace(req.System) != "" {
		params.Instructions = openai.String(req.System)
	}
	if maxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(maxTokens))
	}
	if req.Temperature != 0 {
		params.Temperature = openai.Float(req.Temperature)
	}
	if req.JSONOnly {
		rf := shared.NewResponseFormatJSONObjectParam()
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &rf,
			},
		}
	}

	// Request reasoning summaries when supported.
	params.Reasoning = shared.ReasoningParam{
		GenerateSummary: shared.ReasoningGenerateSummaryAuto, // deprecated but harmless; helps compat
		Summary:         shared.ReasoningSummaryAuto,
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

func (c *Client) toResponseFromResponses(resp *responses.Response) (types.LLMResponse, error) {
	if resp == nil {
		return types.LLMResponse{}, fmt.Errorf("response is nil")
	}

	text := strings.TrimSpace(resp.OutputText())
	out := types.LLMResponse{Text: text}

	// Preserve response ID for follow-up calls via previous_response_id.
	if strings.TrimSpace(resp.ID) != "" {
		out.ResponseID = resp.ID
	}

	if raw := strings.TrimSpace(resp.RawJSON()); raw != "" {
		out.Raw = json.RawMessage(raw)
	}

	// Usage is required by the Responses API, but some providers may still return zeros.
	if resp.Usage.TotalTokens != 0 || resp.Usage.InputTokens != 0 || resp.Usage.OutputTokens != 0 {
		out.Usage = &types.LLMUsage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
			TotalTokens:  int(resp.Usage.TotalTokens),
		}
	}

	return out, nil
}

func (c *Client) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}

	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateResponses(ctx, req); err == nil {
		return out, nil
	} else if shouldFallbackToChat(err) {
		return c.generateChat(ctx, req)
	} else {
		return types.LLMResponse{}, err
	}
}

func (c *Client) generateResponses(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	params, err := c.buildResponseParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}
	resp, err := c.client.Responses.New(ctx, params)
	if err != nil {
		return types.LLMResponse{}, err
	}
	return c.toResponseFromResponses(resp)
}

func (c *Client) generateChat(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
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

func (c *Client) onResponsesStreamEvent(ev responses.ResponseStreamEventUnion, cb types.LLMStreamCallback, outText *strings.Builder, completed **responses.Response) error {
	if cb == nil {
		// Still track completion for final response mapping when desired.
		switch e := ev.AsAny().(type) {
		case responses.ResponseCompletedEvent:
			if completed != nil {
				r := e.Response
				*completed = &r
			}
		}
		return nil
	}

	switch e := ev.AsAny().(type) {
	case responses.ResponseTextDeltaEvent:
		if outText != nil {
			outText.WriteString(e.Delta)
		}
		return cb(types.LLMStreamChunk{Text: e.Delta})
	case responses.ResponseReasoningSummaryTextDeltaEvent:
		// Provider-supplied reasoning summary (safe to show).
		return cb(types.LLMStreamChunk{IsReasoning: true, Text: e.Delta})
	case responses.ResponseReasoningTextDeltaEvent:
		// Raw reasoning (never show): indicator only.
		return cb(types.LLMStreamChunk{IsReasoning: true})
	case responses.ResponseCompletedEvent:
		if completed != nil {
			r := e.Response
			*completed = &r
		}
		return nil
	default:
		return nil
	}
}

func (c *Client) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.client == nil {
		return types.LLMResponse{}, fmt.Errorf("llm client is nil")
	}
	// Prefer Responses API (enables reasoning summaries) and fall back to Chat Completions.
	if out, err := c.generateStreamResponses(ctx, req, cb); err == nil {
		return out, nil
	} else if shouldFallbackToChat(err) {
		return c.generateStreamChat(ctx, req, cb)
	} else {
		return types.LLMResponse{}, err
	}
}

func (c *Client) generateStreamResponses(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	params, err := c.buildResponseParams(req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	stream := c.client.Responses.NewStreaming(ctx, params)
	if stream == nil {
		return types.LLMResponse{}, fmt.Errorf("stream is nil")
	}

	var outText strings.Builder
	var completed *responses.Response

	for stream.Next() {
		ev := stream.Current()
		if err := c.onResponsesStreamEvent(ev, cb, &outText, &completed); err != nil {
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

	if completed != nil {
		return c.toResponseFromResponses(completed)
	}
	// Fallback: return whatever we observed as output text.
	return types.LLMResponse{Text: strings.TrimSpace(outText.String())}, nil
}

func (c *Client) generateStreamChat(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
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

func shouldFallbackToChat(err error) bool {
	if err == nil {
		return false
	}
	var apierr *openai.Error
	if errors.As(err, &apierr) {
		// Common: OpenRouter/provider doesn't support /responses.
		if apierr.StatusCode == 404 {
			return true
		}
	}
	// Conservative string matching for unknown routes/unsupported params.
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, " responses"),
		strings.Contains(s, "/responses"),
		strings.Contains(s, "not found"),
		strings.Contains(s, "unknown route"),
		strings.Contains(s, "unknown endpoint"),
		strings.Contains(s, "unsupported"):
		return true
	default:
		return false
	}
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
