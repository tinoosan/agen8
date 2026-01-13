package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/types"
)

// OpenRouterClient implements types.LLMClient using OpenRouter's OpenAI-compatible
// Chat Completions endpoint.
//
// v0 scope:
//   - POST https://openrouter.ai/api/v1/chat/completions
//   - no streaming
//   - no tool/function calling
//   - best-effort JSON-only mode via response_format
//
// Environment variables (used by NewOpenRouterClientFromEnv):
//   - OPENROUTER_API_KEY (required)
//   - OPENROUTER_BASE_URL (optional, default https://openrouter.ai/api/v1)
//   - OPENROUTER_APP_URL (optional, sent as HTTP-Referer)
//   - OPENROUTER_APP_TITLE (optional, sent as X-Title)
type OpenRouterClient struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client

	// Optional "nice to have" headers for OpenRouter analytics.
	AppURL   string // sent as HTTP-Referer
	AppTitle string // sent as X-Title

	// DefaultMaxTokens is used when LLMRequest.MaxTokens is 0.
	// Keeping this bounded avoids unexpectedly large (and expensive) generations.
	DefaultMaxTokens int
}

func NewOpenRouterClientFromEnv() (*OpenRouterClient, error) {
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

	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &OpenRouterClient{
		APIKey:           key,
		BaseURL:          strings.TrimRight(baseURL, "/"),
		HTTP:             httpClient,
		AppURL:           strings.TrimSpace(os.Getenv("OPENROUTER_APP_URL")),
		AppTitle:         strings.TrimSpace(os.Getenv("OPENROUTER_APP_TITLE")),
		DefaultMaxTokens: defaultMaxTokens,
	}, nil
}

func (c *OpenRouterClient) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil {
		return types.LLMResponse{}, fmt.Errorf("OpenRouterClient is nil")
	}
	if c.APIKey == "" {
		return types.LLMResponse{}, fmt.Errorf("OpenRouterClient APIKey is required")
	}
	if req.Model == "" {
		return types.LLMResponse{}, fmt.Errorf("model is required")
	}
	if c.BaseURL == "" {
		return types.LLMResponse{}, fmt.Errorf("OpenRouterClient BaseURL is required")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type responseFormat struct {
		Type string `json:"type"`
	}
	type requestBody struct {
		Model       string          `json:"model"`
		Messages    []message       `json:"messages"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		Temperature *float64        `json:"temperature,omitempty"`
		ResponseFmt *responseFormat `json:"response_format,omitempty"`
	}

	msgs := make([]message, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, message{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "user"
		}
		msgs = append(msgs, message{Role: role, Content: m.Content})
	}

	var tempPtr *float64
	if req.Temperature != 0 {
		t := req.Temperature
		tempPtr = &t
	}

	var respFmt *responseFormat
	if req.JSONOnly {
		respFmt = &responseFormat{Type: "json_object"}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.DefaultMaxTokens
	}
	if maxTokens < 0 {
		return types.LLMResponse{}, fmt.Errorf("maxTokens must be >= 0")
	}

	body := requestBody{
		Model:       req.Model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: tempPtr,
		ResponseFmt: respFmt,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return types.LLMResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	u := c.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return types.LLMResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	if c.AppURL != "" {
		httpReq.Header.Set("HTTP-Referer", c.AppURL)
	}
	if c.AppTitle != "" {
		httpReq.Header.Set("X-Title", c.AppTitle)
	}

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return types.LLMResponse{}, fmt.Errorf("openrouter request failed: %w", err)
	}
	defer httpResp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(httpResp.Body, 2*1024*1024))
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return types.LLMResponse{}, fmt.Errorf("openrouter status %s: %s", httpResp.Status, strings.TrimSpace(string(raw)))
	}

	type choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	type usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}
	type responseBody struct {
		Choices []choice `json:"choices"`
		Usage   *usage   `json:"usage,omitempty"`
	}

	var parsed responseBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return types.LLMResponse{}, fmt.Errorf("parse openrouter response: %w; raw=%s", err, strings.TrimSpace(string(raw)))
	}
	if len(parsed.Choices) == 0 {
		return types.LLMResponse{}, fmt.Errorf("openrouter response missing choices; raw=%s", strings.TrimSpace(string(raw)))
	}

	out := types.LLMResponse{
		Text: strings.TrimSpace(parsed.Choices[0].Message.Content),
		Raw:  raw,
	}
	if parsed.Usage != nil {
		out.Usage = &types.LLMUsage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
			TotalTokens:  parsed.Usage.TotalTokens,
		}
	}
	return out, nil
}
