package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tinoosan/workbench-core/pkg/llm/types"
)

type testRoleMapper struct {
	fn func(raw string) (CanonicalRole, error)
}

func (m testRoleMapper) Canonicalize(raw string) (CanonicalRole, error) {
	if m.fn == nil {
		return "", fmt.Errorf("mapper fn is nil")
	}
	return m.fn(raw)
}

func TestNewClientFromEnv_RequiresAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_BASE_URL", "")
	_, err := NewClientFromEnv()
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestDefaultRoleMapper_Canonicalize(t *testing.T) {
	mapper := defaultRoleMapper{}
	cases := []struct {
		raw  string
		want CanonicalRole
	}{
		{raw: "user", want: RoleUser},
		{raw: " system ", want: RoleSystem},
		{raw: "DEVELOPER", want: RoleDeveloper},
		{raw: "assistant", want: RoleAssistant},
		{raw: "tool", want: RoleTool},
		{raw: "compaction", want: RoleCompaction},
	}
	for _, tc := range cases {
		got, err := mapper.Canonicalize(tc.raw)
		if err != nil {
			t.Fatalf("Canonicalize(%q): %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("Canonicalize(%q)=%q, want %q", tc.raw, got, tc.want)
		}
	}

	if _, err := mapper.Canonicalize(""); err == nil {
		t.Fatalf("expected error for empty role")
	}
	if _, err := mapper.Canonicalize("unknown"); err == nil {
		t.Fatalf("expected error for unknown role")
	}
}

func TestNewClientFromEnv_UsesDefaultExtensions(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "k")
	t.Setenv("OPENROUTER_BASE_URL", "http://example")

	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	if c.roleMapper == nil {
		t.Fatalf("expected default role mapper")
	}
	if len(c.streamEventHandlers) == 0 {
		t.Fatalf("expected default stream event handlers")
	}
}

func TestNewClientFromEnvWithConfig_InstallsCustomMapperAndHandlers(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "k")
	t.Setenv("OPENROUTER_BASE_URL", "http://example")

	mapper := testRoleMapper{fn: func(raw string) (CanonicalRole, error) {
		if strings.EqualFold(strings.TrimSpace(raw), "analyst") {
			return RoleUser, nil
		}
		return defaultRoleMapper{}.Canonicalize(raw)
	}}
	customHandlerCalled := false
	customHandler := streamEventHandlerFunc{
		eventType: "response.custom.delta",
		fn: func(ev responses.ResponseStreamEventUnion, ctx *ResponsesStreamEventContext) (bool, error) {
			customHandlerCalled = true
			if ctx != nil && ctx.Callback != nil {
				if err := ctx.Callback(types.LLMStreamChunk{Text: "custom"}); err != nil {
					return true, err
				}
			}
			return true, nil
		},
	}

	c, err := NewClientFromEnvWithConfig(OpenAIClientConfig{
		RoleMapper:          mapper,
		StreamEventHandlers: []StreamEventHandler{customHandler},
	})
	if err != nil {
		t.Fatalf("NewClientFromEnvWithConfig: %v", err)
	}

	if _, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.LLMMessage{{Role: "analyst", Content: "hi"}},
	}); err != nil {
		t.Fatalf("expected custom role mapper to allow analyst role: %v", err)
	}

	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{"type":"response.custom.delta","delta":"x"}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	var got []types.LLMStreamChunk
	if err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, nil); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if !customHandlerCalled {
		t.Fatalf("expected custom stream event handler to be called")
	}
	if len(got) != 1 || got[0].Text != "custom" {
		t.Fatalf("chunks=%+v", got)
	}
}

func TestClient_buildParams_MapsSystemMessagesJSONOnly(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "ok"}},
		JSONOnly: true,
	})
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	msgs, _ := m["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// first is system injected from req.System
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "system" {
		t.Fatalf("expected system role, got %+v", m0)
	}

	rf, _ := m["response_format"].(map[string]any)
	if rf == nil || rf["type"] != "json_object" {
		t.Fatalf("expected response_format json_object, got %+v", m["response_format"])
	}
	if _, ok := m["max_tokens"]; !ok {
		t.Fatalf("expected max_tokens to be set from default")
	}
}

func TestClient_buildParams_UsesJSONSchemaWhenProvided(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		JSONOnly: true,
		ResponseSchema: &types.LLMResponseSchema{
			Name:   "test_schema",
			Strict: true,
			Schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"op": map[string]any{"type": "string"},
				},
				"required": []any{"op"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rf, _ := m["response_format"].(map[string]any)
	if rf == nil || rf["type"] != "json_schema" {
		t.Fatalf("expected response_format json_schema, got %+v", m["response_format"])
	}
	js, _ := rf["json_schema"].(map[string]any)
	if js == nil || js["name"] != "test_schema" {
		t.Fatalf("expected response_format.json_schema.name=test_schema, got %+v", js)
	}
}

func TestClient_buildParams_UnknownRoleReturnsError(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.LLMMessage{{Role: "unknown-role", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error for unknown role")
	}
}

func TestClient_toResponse_MapsTextAndUsage(t *testing.T) {
	resp := &openai.ChatCompletion{
		Model: "openai/gpt-5-mini-2026-01-01",
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: " ok "}},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     3,
			CompletionTokens: 4,
			TotalTokens:      7,
			CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
				ReasoningTokens: 2,
			},
		},
	}

	c := &Client{}
	got, err := c.toResponse(resp)
	if err != nil {
		t.Fatalf("toResponse: %v", err)
	}
	if got.Text != "ok" {
		t.Fatalf("text = %q", got.Text)
	}
	if got.Usage == nil || got.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if got.Usage.ReasoningTokens != 2 {
		t.Fatalf("reasoning tokens = %d, want %d", got.Usage.ReasoningTokens, 2)
	}
	if got.EffectiveModel != "openai/gpt-5-mini-2026-01-01" {
		t.Fatalf("effective model = %q", got.EffectiveModel)
	}
}

func TestClient_buildResponseParams_JSONSchema(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5-mini",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		ResponseSchema: &types.LLMResponseSchema{
			Name:   "test_schema",
			Strict: true,
			Schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"op": map[string]any{"type": "string"},
				},
				"required": []any{"op"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	if params.Text.Format.OfJSONSchema == nil {
		t.Fatalf("expected JSON schema response format")
	}
}

func TestClient_buildResponseParams_UnknownRoleReturnsError(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5-mini",
		Messages: []types.LLMMessage{{Role: "unknown-role", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("expected error for unknown role")
	}
}

func TestClient_buildResponseParams_ReasoningSummaryNoneMapsToOff(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:            "openai/gpt-5-nano",
		System:           "system",
		Messages:         []types.LLMMessage{{Role: "user", Content: "hi"}},
		ReasoningSummary: "none",
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	if string(params.Reasoning.Summary) != "" {
		t.Fatalf("expected summary omitted for none/off, got %+v", params.Reasoning.Summary)
	}
}

func TestClient_buildResponseParams_InvalidReasoningSummaryFallsBackToAuto(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:            "openai/gpt-5-nano",
		System:           "system",
		Messages:         []types.LLMMessage{{Role: "user", Content: "hi"}},
		ReasoningSummary: "verbose",
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	if string(params.Reasoning.Summary) != "auto" {
		t.Fatalf("expected summary auto fallback, got %+v", params.Reasoning.Summary)
	}
	if string(params.Reasoning.GenerateSummary) != "" {
		t.Fatalf("expected deprecated generate_summary to be omitted, got %+v", params.Reasoning.GenerateSummary)
	}
}

func TestClient_buildResponseParams_ReasoningSummaryAutoForOnlineVariant(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5-nano:online",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	if string(params.Reasoning.Summary) != "auto" {
		t.Fatalf("expected summary auto for model variant, got %+v", params.Reasoning.Summary)
	}
	if string(params.Reasoning.GenerateSummary) != "" {
		t.Fatalf("expected deprecated generate_summary to be omitted, got %+v", params.Reasoning.GenerateSummary)
	}
}

func TestClient_toResponseFromResponses_MapsToolCallsAndUsage(t *testing.T) {
	resp := &responses.Response{
		Model: responses.ResponsesModelGPT5Pro,
		Output: []responses.ResponseOutputItemUnion{
			responses.ResponseOutputItemUnion{Type: "function_call", CallID: "cid", Name: "tool", Arguments: "{}"},
		},
		Usage: responses.ResponseUsage{
			InputTokens:  5,
			OutputTokens: 6,
			TotalTokens:  11,
			OutputTokensDetails: responses.ResponseUsageOutputTokensDetails{
				ReasoningTokens: 3,
			},
		},
	}
	c := &Client{}
	got, err := c.toResponseFromResponses(resp)
	if err != nil {
		t.Fatalf("toResponseFromResponses: %v", err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != "cid" {
		t.Fatalf("toolCalls = %+v", got.ToolCalls)
	}
	if got.Usage == nil || got.Usage.TotalTokens != 11 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	if got.Usage.ReasoningTokens != 3 {
		t.Fatalf("reasoning tokens = %d, want %d", got.Usage.ReasoningTokens, 3)
	}
	if got.EffectiveModel == "" {
		t.Fatalf("expected effective model")
	}
}

func TestOnResponsesStreamEvent_EmitsReasoningSummaryFromOutputItemDone(t *testing.T) {
	raw := `{
		"type":"response.output_item.done",
		"sequence_number":1,
		"output_index":0,
		"item":{
			"id":"rs_1",
			"type":"reasoning",
			"summary":[{"type":"summary_text","text":"Done-event summary"}]
		}
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	saw := false
	var got []types.LLMStreamChunk
	err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, &saw)
	if err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("chunks = %d, want 1", len(got))
	}
	if !got[0].IsReasoning || got[0].Text != "Done-event summary" {
		t.Fatalf("chunk = %+v", got[0])
	}
	if !saw {
		t.Fatalf("expected sawReasoningSummaryText=true")
	}
}

func TestOnResponsesStreamEvent_SkipsOutputItemDoneSummaryWhenAlreadySeen(t *testing.T) {
	raw := `{
		"type":"response.output_item.done",
		"sequence_number":1,
		"output_index":0,
		"item":{
			"id":"rs_1",
			"type":"reasoning",
			"summary":[{"type":"summary_text","text":"Done-event summary"}]
		}
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	saw := true
	var got []types.LLMStreamChunk
	err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, &saw)
	if err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("chunks = %d, want 0", len(got))
	}
}

func TestOnResponsesStreamEvent_EmitsReasoningSummaryFromUnknownRawEvent(t *testing.T) {
	raw := `{
		"type":"response.reasoning_summary.delta",
		"sequence_number":1,
		"delta":"raw summary variant"
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	saw := false
	var got []types.LLMStreamChunk
	err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, &saw)
	if err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("chunks = %d, want 1", len(got))
	}
	if !got[0].IsReasoning || got[0].Text != "raw summary variant" {
		t.Fatalf("chunk = %+v", got[0])
	}
	if !saw {
		t.Fatalf("expected sawReasoningSummaryText=true")
	}
}

func TestOnResponsesStreamEvent_UnknownNonReasoningEventNoOp(t *testing.T) {
	raw := `{
		"type":"response.unknown.event",
		"sequence_number":1,
		"delta":"noop"
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	var got []types.LLMStreamChunk
	err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("chunks = %d, want 0", len(got))
	}
}

func TestOnResponsesStreamEvent_CallbackNilTracksCompleted(t *testing.T) {
	raw := `{
		"type":"response.completed",
		"response":{"id":"resp_123","output":[]}
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	var completed *responses.Response
	if err := c.onResponsesStreamEvent(ev, nil, nil, &completed, nil); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if completed == nil || strings.TrimSpace(completed.ID) != "resp_123" {
		t.Fatalf("completed = %+v", completed)
	}
}

func TestOnStreamChunk_EmitsStructuredReasoningSummary(t *testing.T) {
	raw := `{
		"id":"chatcmpl-1",
		"object":"chat.completion.chunk",
		"created":123,
		"model":"openai/gpt-5-nano",
		"choices":[{
			"index":0,
			"delta":{
				"reasoning_summary":[
					{"type":"summary_text","text":"first summary"},
					{"type":"summary_text","text":"second summary"}
				]
			}
		}]
	}`
	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}

	c := &Client{}
	var got []types.LLMStreamChunk
	if err := c.onStreamChunk(nil, chunk, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}); err != nil {
		t.Fatalf("onStreamChunk: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunks = %d, want 2", len(got))
	}
	if !got[0].IsReasoning || got[0].Text != "first summary" {
		t.Fatalf("first chunk = %+v", got[0])
	}
	if !got[1].IsReasoning || got[1].Text != "second summary" {
		t.Fatalf("second chunk = %+v", got[1])
	}
}

func TestOnStreamChunk_EmitsReasoningSummaryFromReasoningDetails(t *testing.T) {
	raw := `{
		"id":"chatcmpl-1",
		"object":"chat.completion.chunk",
		"created":123,
		"model":"openai/gpt-5-nano",
		"choices":[{
			"index":0,
			"delta":{
				"reasoning_details":[
					{"type":"reasoning_text","text":"hidden raw reasoning"},
					{"type":"summary_text","text":"summary from reasoning details"}
				]
			}
		}]
	}`
	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}

	c := &Client{}
	var got []types.LLMStreamChunk
	if err := c.onStreamChunk(nil, chunk, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}); err != nil {
		t.Fatalf("onStreamChunk: %v", err)
	}
	textChunks := make([]types.LLMStreamChunk, 0, len(got))
	for _, ch := range got {
		if strings.TrimSpace(ch.Text) != "" {
			textChunks = append(textChunks, ch)
		}
	}
	if len(textChunks) != 1 {
		t.Fatalf("text chunks = %d, want 1 (all chunks=%+v)", len(textChunks), got)
	}
	if !textChunks[0].IsReasoning || textChunks[0].Text != "summary from reasoning details" {
		t.Fatalf("chunk = %+v", textChunks[0])
	}
}

func TestResponseReasoningSummaryTextsFromResponse(t *testing.T) {
	resp := &responses.Response{
		Output: []responses.ResponseOutputItemUnion{
			{
				Type: "reasoning",
				Summary: []responses.ResponseReasoningItemSummary{
					{Type: "summary_text", Text: "final summary from completed response"},
				},
			},
		},
	}
	got := responseReasoningSummaryTextsFromResponse(resp)
	if len(got) != 1 {
		t.Fatalf("summaries = %d, want 1", len(got))
	}
	if got[0] != "final summary from completed response" {
		t.Fatalf("summary = %q", got[0])
	}
}

func TestResponseReasoningSummaryTextsFromResponse_FallsBackToRawOutput(t *testing.T) {
	var resp responses.Response
	if err := json.Unmarshal([]byte(`{
		"output":[
			{
				"type":"reasoning",
				"summary":[{"type":"summary_text","text":"summary from raw response"}]
			}
		]
	}`), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got := responseReasoningSummaryTextsFromResponse(&resp)
	if len(got) != 1 {
		t.Fatalf("summaries = %d, want 1", len(got))
	}
	if got[0] != "summary from raw response" {
		t.Fatalf("summary = %q", got[0])
	}
}

func TestShouldFallbackToChatForRequest_DisablesFallbackForOpenAIBaseURL(t *testing.T) {
	req := types.LLMRequest{Model: "openai/gpt-5-nano"}
	err := &openai.Error{StatusCode: 404}
	if shouldFallbackToChatForRequest("https://api.openai.com/v1", req, err) {
		t.Fatalf("expected no fallback for OpenAI base URL")
	}
}

func TestShouldFallbackToChatForRequest_DisablesFallbackForReasoningModel(t *testing.T) {
	req := types.LLMRequest{Model: "openai/gpt-5-nano:online"}
	err := &openai.Error{StatusCode: 404}
	if shouldFallbackToChatForRequest("https://openrouter.ai/api/v1", req, err) {
		t.Fatalf("expected no fallback for reasoning model")
	}
}

func TestShouldFallbackToChatForRequest_DisablesFallbackForOpenRouterPolicyError(t *testing.T) {
	req := types.LLMRequest{Model: "openai/gpt-oss-120b:free"}
	err := &openai.Error{StatusCode: 404, Message: "No endpoints found matching your data policy (Free model publication). Configure: https://openrouter.ai/settings/privacy"}
	if shouldFallbackToChatForRequest("https://openrouter.ai/api/v1", req, err) {
		t.Fatalf("expected no fallback for OpenRouter data policy error")
	}
}

func TestShouldFallbackToChat_404(t *testing.T) {
	err := &openai.Error{StatusCode: 404}
	if !shouldFallbackToChat(err) {
		t.Fatalf("expected fallback for 404")
	}
}

func TestShouldAllowOpenRouterFreeModelDataCollection_DefaultAndEnv(t *testing.T) {
	t.Setenv("OPENROUTER_FREE_MODEL_DATA_COLLECTION", "")
	if !shouldAllowOpenRouterFreeModelDataCollection("openai/gpt-oss-120b:free") {
		t.Fatalf("expected default allow for :free model")
	}
	if shouldAllowOpenRouterFreeModelDataCollection("openai/gpt-5-nano") {
		t.Fatalf("expected non-free model to remain disabled")
	}

	t.Setenv("OPENROUTER_FREE_MODEL_DATA_COLLECTION", "false")
	if shouldAllowOpenRouterFreeModelDataCollection("openai/gpt-oss-120b:free") {
		t.Fatalf("expected env override to disable free-model data collection")
	}
}

func TestShouldRetryOpenRouterFreeModelPolicy(t *testing.T) {
	err := &openai.Error{StatusCode: 404, Message: "No endpoints found matching your data policy (Free model publication). Configure: https://openrouter.ai/settings/privacy"}
	if !shouldRetryOpenRouterFreeModelPolicy("https://openrouter.ai/api/v1", "openai/gpt-oss-120b:free", err) {
		t.Fatalf("expected retry for OpenRouter free-model data-policy error")
	}
	if shouldRetryOpenRouterFreeModelPolicy("https://openrouter.ai/api/v1", "openai/gpt-5-nano", err) {
		t.Fatalf("did not expect retry for non-free model")
	}
	if shouldRetryOpenRouterFreeModelPolicy("https://api.openai.com/v1", "openai/gpt-oss-120b:free", err) {
		t.Fatalf("did not expect retry outside OpenRouter base URL")
	}
}

func TestShouldFallbackFromJSONSchema_400(t *testing.T) {
	err := &openai.Error{StatusCode: 400, Message: "response_format json_schema unsupported"}
	if !shouldFallbackFromJSONSchema(err) {
		t.Fatalf("expected fallback for json_schema")
	}
}

func TestExtractURLCitationsFromResponsesRaw(t *testing.T) {
	raw := json.RawMessage(`{"output":[{"type":"message","content":[{"type":"output_text","text":"hi","annotations":[{"type":"url_citation","url":"https://example.com","title":"Example"}]}]}]}`)
	got, err := extractURLCitationsFromResponsesRaw(raw)
	if err != nil {
		t.Fatalf("extractURLCitationsFromResponsesRaw: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com" {
		t.Fatalf("citations = %+v", got)
	}
}

func TestBuildParams_ToolChoiceRequired(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		Tools: []types.Tool{
			{Type: "function", Function: types.ToolFunction{Name: "t"}},
		},
		ToolChoice: "function:",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildParams_ToolChoiceFunction(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		Tools: []types.Tool{
			{Type: "function", Function: types.ToolFunction{Name: "t"}},
		},
		ToolChoice: "function:t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildParams_UnsupportedToolType(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildParams(types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		Tools: []types.Tool{
			{Type: "notfunc", Function: types.ToolFunction{Name: "t"}},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildResponseParams_ToolChoiceFunction(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5-mini",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		Tools: []types.Tool{
			{Type: "function", Function: types.ToolFunction{Name: "t"}},
		},
		ToolChoice: "function:t",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildResponseParams_UnsupportedToolType(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5-mini",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		Tools: []types.Tool{
			{Type: "notfunc", Function: types.ToolFunction{Name: "t"}},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildResponseParams_CompactionMessageMapped(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	params, err := c.buildResponseParams(types.LLMRequest{
		Model: "openai/gpt-5-mini",
		Messages: []types.LLMMessage{
			{Role: "user", Content: "hello"},
			{Role: "compaction", Content: "encrypted_payload"},
		},
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	input, _ := m["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input len=%d, want 2", len(input))
	}
	last, _ := input[1].(map[string]any)
	if last["type"] != "compaction" {
		t.Fatalf("expected second input item type=compaction, got %+v", last)
	}
}

func TestClient_CompactConversation_UnknownRoleReturnsError(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli}
	_, err := c.CompactConversation(nil, types.LLMCompactionRequest{
		Model: "openai/gpt-5-mini",
		Messages: []types.LLMMessage{
			{Role: "unknown-role", Content: "x"},
		},
	})
	if err == nil {
		t.Fatalf("expected error for unknown role")
	}
}

func TestShouldFallbackFromJSONSchema_ReturnsFalseForNonSchemaErrors(t *testing.T) {
	if shouldFallbackFromJSONSchema(errors.New("other error")) {
		t.Fatalf("expected no fallback for non-schema error")
	}
}

func TestMaybeEnableWebSearchModel(t *testing.T) {
	if got := maybeEnableWebSearchModel("https://openrouter.ai/api/v1", "openai/gpt-4o", true); !strings.HasSuffix(got, ":online") {
		t.Fatalf("expected :online suffix, got %q", got)
	}
	if got := maybeEnableWebSearchModel("https://example.com", "openai/gpt-4o", true); got != "openai/gpt-4o" {
		t.Fatalf("expected unchanged model, got %q", got)
	}
}

func TestResponseFormatKindResponses(t *testing.T) {
	if responseFormatKindResponses(responses.ResponseNewParams{}) != "none" {
		t.Fatalf("expected none for empty params")
	}
}

func TestResponseFormatKindChat(t *testing.T) {
	if responseFormatKindChat(openai.ChatCompletionNewParams{}) != "none" {
		t.Fatalf("expected none for empty params")
	}
}

func TestClient_buildParams_DropsSchemaAfterUnsupported(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}
	c.schemaUnsupported.Store(true)
	params, err := c.buildParams(types.LLMRequest{
		Model:          "openai/gpt-4o-mini",
		System:         "system",
		Messages:       []types.LLMMessage{{Role: "user", Content: "hi"}},
		ResponseSchema: &types.LLMResponseSchema{Name: "x", Schema: map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if params.ResponseFormat.OfJSONSchema != nil {
		t.Fatalf("expected schema to be dropped when unsupported")
	}
}

func TestClient_buildResponseParams_DropsSchemaAfterUnsupported(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}
	c.schemaUnsupported.Store(true)
	params, err := c.buildResponseParams(types.LLMRequest{
		Model:          "openai/gpt-5-mini",
		System:         "system",
		Messages:       []types.LLMMessage{{Role: "user", Content: "hi"}},
		ResponseSchema: &types.LLMResponseSchema{Name: "x", Schema: map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatalf("buildResponseParams: %v", err)
	}
	if params.Text.Format.OfJSONSchema != nil {
		t.Fatalf("expected schema to be dropped when unsupported")
	}
}

func TestOnResponsesStreamEvent_OutputItemDoneAddsSeparatorBetweenSummaryParts(t *testing.T) {
	raw := `{
		"type":"response.output_item.done",
		"sequence_number":1,
		"output_index":0,
		"item":{
			"id":"rs_1",
			"type":"reasoning",
			"summary":[
				{"type":"summary_text","text":"First section"},
				{"type":"summary_text","text":"Second section"}
			]
		}
	}`
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	c := &Client{}
	saw := false
	var got []types.LLMStreamChunk
	err := c.onResponsesStreamEvent(ev, func(ch types.LLMStreamChunk) error {
		got = append(got, ch)
		return nil
	}, nil, nil, &saw)
	if err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunks = %d, want 2", len(got))
	}
	if got[0].Text != "First section" {
		t.Fatalf("first summary = %q", got[0].Text)
	}
	if got[1].Text != "\n\nSecond section" {
		t.Fatalf("second summary = %q", got[1].Text)
	}
}
