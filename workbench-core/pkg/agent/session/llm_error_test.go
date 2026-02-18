package session

import (
	"testing"

	"github.com/tinoosan/workbench-core/pkg/llm"
)

func TestLLMErrorMessage(t *testing.T) {
	cases := []struct {
		class string
		want  string
	}{
		{class: "quota", want: "LLM quota/credits exhausted"},
		{class: "rate_limit", want: "LLM rate limit reached"},
		{class: "network", want: "LLM network error"},
		{class: "timeout", want: "LLM request timed out"},
		{class: "auth", want: "LLM authentication failed"},
		{class: "permission", want: "LLM permission denied"},
		{class: "policy", want: "LLM request blocked by provider data policy (check OpenRouter privacy settings for free models)"},
		{class: "server", want: "LLM provider server error"},
		{class: "invalid_request", want: "LLM request rejected"},
		{class: "unknown", want: "LLM request failed"},
		{class: "", want: "LLM request failed"},
	}
	for _, tc := range cases {
		got := llmErrorMessage(llm.ErrorInfo{Class: tc.class})
		if got != tc.want {
			t.Fatalf("llmErrorMessage(%q)=%q want %q", tc.class, got, tc.want)
		}
	}
}
