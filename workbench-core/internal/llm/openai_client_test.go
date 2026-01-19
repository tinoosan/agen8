package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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

func TestClient_onStreamChunk_ForwardsDeltaContent(t *testing.T) {
	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(`{
	  "id":"x",
	  "object":"chat.completion.chunk",
	  "created":0,
	  "model":"m",
	  "choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":""}]
	}`), &chunk); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}

	var got string
	c := &Client{}
	var acc openai.ChatCompletionAccumulator
	if err := c.onStreamChunk(&acc, chunk, func(sc types.LLMStreamChunk) error {
		got += sc.Text
		return nil
	}); err != nil {
		t.Fatalf("onStreamChunk: %v", err)
	}
	if got != "hi" {
		t.Fatalf("unexpected got %q", got)
	}
}

