package llm

import (
	"errors"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tinoosan/workbench-core/internal/types"
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
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: " ok "}},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	c := &Client{}
	out, err := c.toResponse(resp)
	if err != nil {
		t.Fatalf("toResponse: %v", err)
	}
	if out.Text != "ok" {
		t.Fatalf("unexpected text %q", out.Text)
	}
	if out.Usage == nil || out.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage %+v", out.Usage)
	}
}

func TestClient_buildResponseParams_MapsInstructionsJSONOnlyAndReasoningSummaryAuto(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5.1-codex-mini",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "ok"}},
		JSONOnly: true,
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

	if m["instructions"] != "system" {
		t.Fatalf("expected instructions=system, got %+v", m["instructions"])
	}
	if _, ok := m["reasoning"].(map[string]any); !ok {
		t.Fatalf("expected reasoning object, got %+v", m["reasoning"])
	}
	txt, _ := m["text"].(map[string]any)
	if txt == nil {
		t.Fatalf("expected text config")
	}
	format, _ := txt["format"].(map[string]any)
	if format == nil || format["type"] != "json_object" {
		t.Fatalf("expected text.format json_object, got %+v", txt["format"])
	}

	// Default: no previous_response_id.
	if _, ok := m["previous_response_id"]; ok {
		t.Fatalf("expected previous_response_id to be omitted, got %+v", m["previous_response_id"])
	}
}

func TestClient_buildResponseParams_UsesJSONSchemaWhenProvided(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:    "openai/gpt-5.1-codex-mini",
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

	txt, _ := m["text"].(map[string]any)
	if txt == nil {
		t.Fatalf("expected text config")
	}
	format, _ := txt["format"].(map[string]any)
	if format == nil || format["type"] != "json_schema" {
		t.Fatalf("expected text.format json_schema, got %+v", txt["format"])
	}
	if format["name"] != "test_schema" {
		t.Fatalf("expected text.format.name=test_schema, got %+v", format["name"])
	}
}

func TestClient_buildResponseParams_IncludesPreviousResponseIDAndDeltaOnlyInput(t *testing.T) {
	cli := openai.NewClient(option.WithAPIKey("k"), option.WithBaseURL("http://example"))
	c := &Client{client: &cli, DefaultMaxTokens: 123}

	params, err := c.buildResponseParams(types.LLMRequest{
		Model:              "openai/gpt-5.1-codex-mini",
		System:             "system",
		PreviousResponseID: "resp_123",
		Messages: []types.LLMMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "ok"},
		},
		JSONOnly: true,
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

	if m["previous_response_id"] != "resp_123" {
		t.Fatalf("expected previous_response_id=resp_123, got %+v", m["previous_response_id"])
	}

	// Delta-only: input should contain only the newest message.
	input, _ := m["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("expected delta-only input length 1, got %d (input=%+v)", len(input), m["input"])
	}
	// Basic sanity: ensure the last message content appears somewhere in the input item JSON.
	input0b, _ := json.Marshal(input[0])
	if !strings.Contains(string(input0b), `"ok"`) {
		t.Fatalf("expected input item to include last message content, got %s", string(input0b))
	}
}

func TestClient_toResponseFromResponses_ExtractsResponseIDAndText(t *testing.T) {
	resp := &responses.Response{
		ID: "resp_123",
		Output: []responses.ResponseOutputItemUnion{
			{
				Type: "message",
				Content: []responses.ResponseOutputMessageContentUnion{
					{Type: "output_text", Text: " ok "},
				},
			},
		},
	}

	c := &Client{}
	out, err := c.toResponseFromResponses(resp)
	if err != nil {
		t.Fatalf("toResponseFromResponses: %v", err)
	}
	if out.Text != "ok" {
		t.Fatalf("unexpected text %q", out.Text)
	}
	if out.ResponseID != "resp_123" {
		t.Fatalf("expected ResponseID resp_123, got %q", out.ResponseID)
	}
}

func TestClient_onResponsesStreamEvent_ForwardsOutputTextDelta(t *testing.T) {
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{
	  "type":"response.output_text.delta",
	  "delta":"hi",
	  "content_index":0,
	  "item_id":"item",
	  "output_index":0,
	  "sequence_number":1,
	  "logprobs":[]
	}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	var got string
	var out strings.Builder
	var saw bool
	c := &Client{}
	if err := c.onResponsesStreamEvent(ev, func(sc types.LLMStreamChunk) error {
		got += sc.Text
		return nil
	}, &out, nil, &saw); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if got != "hi" {
		t.Fatalf("unexpected got %q", got)
	}
	if out.String() != "hi" {
		t.Fatalf("unexpected out %q", out.String())
	}
}

func TestClient_onResponsesStreamEvent_EmitsReasoningSummaryDelta(t *testing.T) {
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{
	  "type":"response.reasoning_summary_text.delta",
	  "delta":"summary",
	  "item_id":"item",
	  "output_index":0,
	  "sequence_number":2,
	  "summary_index":0
	}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	var got []types.LLMStreamChunk
	var saw bool
	c := &Client{}
	if err := c.onResponsesStreamEvent(ev, func(sc types.LLMStreamChunk) error {
		got = append(got, sc)
		return nil
	}, nil, nil, &saw); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 || !got[0].IsReasoning || got[0].Text != "summary" {
		t.Fatalf("unexpected chunks: %+v", got)
	}
}

func TestClient_onResponsesStreamEvent_EmitsReasoningSignalForReasoningTextDelta(t *testing.T) {
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{
	  "type":"response.reasoning_text.delta",
	  "delta":"secret",
	  "content_index":0,
	  "item_id":"item",
	  "output_index":0,
	  "sequence_number":3
	}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	var got []types.LLMStreamChunk
	var saw bool
	c := &Client{}
	if err := c.onResponsesStreamEvent(ev, func(sc types.LLMStreamChunk) error {
		got = append(got, sc)
		return nil
	}, nil, nil, &saw); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 || !got[0].IsReasoning || got[0].Text != "" {
		t.Fatalf("unexpected chunks: %+v", got)
	}
}

func TestClient_onResponsesStreamEvent_EmitsReasoningSummaryPartAdded(t *testing.T) {
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{
	  "type":"response.reasoning_summary_part.added",
	  "item_id":"item",
	  "output_index":0,
	  "sequence_number":4,
	  "summary_index":0,
	  "part":{"type":"summary_text","text":"part summary"}
	}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	var got []types.LLMStreamChunk
	var saw bool
	c := &Client{}
	if err := c.onResponsesStreamEvent(ev, func(sc types.LLMStreamChunk) error {
		got = append(got, sc)
		return nil
	}, nil, nil, &saw); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 || !got[0].IsReasoning || got[0].Text != "part summary" {
		t.Fatalf("unexpected chunks: %+v", got)
	}
	if !saw {
		t.Fatalf("expected sawReasoningSummaryText=true")
	}
}

func TestClient_onResponsesStreamEvent_EmitsReasoningSummaryTextDoneOnlyWhenNoDeltasSeen(t *testing.T) {
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(`{
	  "type":"response.reasoning_summary_text.done",
	  "item_id":"item",
	  "output_index":0,
	  "sequence_number":5,
	  "summary_index":0,
	  "text":"full summary"
	}`), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	var got []types.LLMStreamChunk
	saw := false
	c := &Client{}
	if err := c.onResponsesStreamEvent(ev, func(sc types.LLMStreamChunk) error {
		got = append(got, sc)
		return nil
	}, nil, nil, &saw); err != nil {
		t.Fatalf("onResponsesStreamEvent: %v", err)
	}
	if len(got) != 1 || !got[0].IsReasoning || got[0].Text != "full summary" {
		t.Fatalf("unexpected chunks: %+v", got)
	}
	if !saw {
		t.Fatalf("expected sawReasoningSummaryText=true")
	}
}

func TestShouldFallbackToChat_ResponsesNotFound(t *testing.T) {
	apierr := &openai.Error{StatusCode: http.StatusNotFound}
	if !shouldFallbackToChat(apierr) {
		t.Fatalf("expected fallback")
	}
}

func TestShouldFallbackFromJSONSchema(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain_string_match", err: &openai.Error{StatusCode: 400, Message: "unsupported response_format json_schema"}, want: true},
		{name: "plain_errors_new", err: errors.New("response_format json_schema not supported"), want: true},
		{name: "other_error", err: errors.New("timeout"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldFallbackFromJSONSchema(tt.err); got != tt.want {
				t.Fatalf("want %v got %v", tt.want, got)
			}
		})
	}
}
