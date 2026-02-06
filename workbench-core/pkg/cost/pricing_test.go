package cost

import "testing"

func TestPricingLookup_ExactMatch(t *testing.T) {
	pf := PricingFile{
		Models: map[string]ModelPricing{
			"openai/gpt-5-nano": {InputPerM: 0.05, OutputPerM: 0.4},
		},
	}
	in, out, ok := pf.Lookup("openai/gpt-5-nano")
	if !ok {
		t.Fatalf("expected exact match")
	}
	if in != 0.05 || out != 0.4 {
		t.Fatalf("unexpected pricing: in=%v out=%v", in, out)
	}
}

func TestPricingLookup_SuffixMatch(t *testing.T) {
	pf := PricingFile{
		Models: map[string]ModelPricing{
			"openai/gpt-5-nano": {InputPerM: 0.05, OutputPerM: 0.4},
		},
	}
	in, out, ok := pf.Lookup("gpt-5-nano")
	if !ok {
		t.Fatalf("expected suffix match")
	}
	if in != 0.05 || out != 0.4 {
		t.Fatalf("unexpected pricing: in=%v out=%v", in, out)
	}
}

func TestPricingLookup_CaseInsensitiveSuffixMatch(t *testing.T) {
	pf := PricingFile{
		Models: map[string]ModelPricing{
			"OpenAI/GPT-5-NANO": {InputPerM: 0.05, OutputPerM: 0.4},
		},
	}
	_, _, ok := pf.Lookup("gPt-5-NaNo")
	if !ok {
		t.Fatalf("expected case-insensitive suffix match")
	}
}

func TestPricingLookup_Miss(t *testing.T) {
	pf := PricingFile{
		Models: map[string]ModelPricing{
			"openai/gpt-5-nano": {InputPerM: 0.05, OutputPerM: 0.4},
		},
	}
	_, _, ok := pf.Lookup("not-a-model")
	if ok {
		t.Fatalf("expected miss")
	}
}

func TestSupportsReasoningEffort_SuffixMatch(t *testing.T) {
	if !SupportsReasoningEffort("gpt-5-nano") {
		t.Fatalf("expected suffix model id to be recognized")
	}
}

func TestSupportsReasoningEffort_CaseInsensitiveSuffixMatch(t *testing.T) {
	if !SupportsReasoningEffort("GPT-5-NANO") {
		t.Fatalf("expected case-insensitive suffix model id to be recognized")
	}
}

func TestSupportsReasoningEffort_Miss(t *testing.T) {
	if SupportsReasoningEffort("unknown-model") {
		t.Fatalf("expected unknown model to be unsupported")
	}
}
