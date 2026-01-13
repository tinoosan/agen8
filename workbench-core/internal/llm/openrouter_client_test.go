package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

func TestOpenRouterClient_Generate_ParsesFirstChoice(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content-type %q", ct)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "choices":[{"message":{"content":"{\"op\":\"final\",\"text\":\"ok\"}"}}],
		  "usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer srv.Close()

	c := &OpenRouterClient{
		APIKey:  "k",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}

	resp, err := c.Generate(context.Background(), types.LLMRequest{
		Model:    "openai/gpt-4o-mini",
		System:   "system",
		Messages: []types.LLMMessage{{Role: "user", Content: "hi"}},
		JSONOnly: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("expected Authorization Bearer, got %q", gotAuth)
	}
	if resp.Text != `{"op":"final","text":"ok"}` {
		t.Fatalf("unexpected text %q", resp.Text)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage %+v", resp.Usage)
	}

	if gotBody["model"] != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected model %v", gotBody["model"])
	}
	if gotBody["response_format"] == nil {
		t.Fatalf("expected response_format to be set when JSONOnly=true")
	}
}
