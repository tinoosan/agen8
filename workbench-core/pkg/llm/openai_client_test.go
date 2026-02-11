package llm

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tinoosan/workbench-core/pkg/llm/types"
)

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

func TestShouldFallbackToChat_404(t *testing.T) {
	err := &openai.Error{StatusCode: 404}
	if !shouldFallbackToChat(err) {
		t.Fatalf("expected fallback for 404")
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
