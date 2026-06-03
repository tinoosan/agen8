package types

import "testing"

func TestTokenUsageAdd(t *testing.T) {
	got := TokenUsage{
		InputTokens:  10,
		OutputTokens: 4,
		TotalTokens:  14,
		CostUSD:      0.25,
	}.Add(TokenUsage{
		InputTokens:  3,
		OutputTokens: 2,
		TotalTokens:  5,
		CostUSD:      0.10,
	})

	want := TokenUsage{
		InputTokens:  13,
		OutputTokens: 6,
		TotalTokens:  19,
		CostUSD:      0.35,
	}
	if got != want {
		t.Fatalf("Add() = %+v want %+v", got, want)
	}
}

func TestTokenUsageIsZero(t *testing.T) {
	if !(TokenUsage{}).IsZero() {
		t.Fatalf("zero usage should report IsZero")
	}
	if (TokenUsage{InputTokens: 1}).IsZero() {
		t.Fatalf("non-zero usage should not report IsZero")
	}
}
