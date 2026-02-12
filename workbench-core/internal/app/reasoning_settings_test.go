package app

import (
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestEnsureSessionReasoningForModel_DefaultsForReasoningModel(t *testing.T) {
	sess := types.Session{}
	ensureSessionReasoningForModel(&sess, "openai/gpt-5-nano", "", "")
	if sess.ReasoningEffort != "medium" {
		t.Fatalf("reasoning effort = %q, want %q", sess.ReasoningEffort, "medium")
	}
	if sess.ReasoningSummary != "auto" {
		t.Fatalf("reasoning summary = %q, want %q", sess.ReasoningSummary, "auto")
	}
	pref, ok := sess.ReasoningByModel[modelReasoningKey("openai/gpt-5-nano")]
	if !ok {
		t.Fatalf("expected per-model preference to be stored")
	}
	if pref.Effort != "medium" || pref.Summary != "auto" {
		t.Fatalf("preference = %+v", pref)
	}
}

func TestStoreSessionReasoningPreference_PerModelSticky(t *testing.T) {
	sess := types.Session{ActiveModel: "openai/gpt-5-nano"}
	storeSessionReasoningPreference(&sess, "openai/gpt-5-nano", "high", "detailed")
	storeSessionReasoningPreference(&sess, "openai/gpt-5", "low", "concise")

	eff, sum := sessionReasoningForModel(sess, "openai/gpt-5-nano", "", "")
	if eff != "high" || sum != "detailed" {
		t.Fatalf("gpt-5-nano preference = (%q,%q)", eff, sum)
	}
	eff, sum = sessionReasoningForModel(sess, "openai/gpt-5", "", "")
	if eff != "low" || sum != "concise" {
		t.Fatalf("gpt-5 preference = (%q,%q)", eff, sum)
	}
}
