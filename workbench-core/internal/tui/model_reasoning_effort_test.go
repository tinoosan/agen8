package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/events"
)

func TestModel_RenderInput_ShowsReasoningEffort(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), stubRunner{}, nil)
	m.width = 120
	m.height = 24
	m.modelID = "openai/gpt-5.1-codex-mini"
	m.reasoningEffort = "high"

	out := m.renderInput()
	if !strings.Contains(out, "effort") {
		t.Fatalf("expected renderInput to include effort label; got %q", out)
	}
	if !strings.Contains(out, "high") {
		t.Fatalf("expected renderInput to include effort value; got %q", out)
	}
}

func TestModel_RenderInput_HidesReasoningEffortForNonReasoningModels(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), stubRunner{}, nil)
	m.width = 120
	m.height = 24
	m.modelID = "openai/gpt-4o"
	m.reasoningEffort = "high"

	out := m.renderInput()
	if strings.Contains(out, "effort") {
		t.Fatalf("expected renderInput to hide effort label for non-reasoning model; got %q", out)
	}
	if strings.Contains(out, "high") {
		t.Fatalf("expected renderInput to hide effort value for non-reasoning model; got %q", out)
	}
}

func TestModel_onEvent_UpdatesReasoningEffort(t *testing.T) {
	t.Parallel()

	m := New(context.Background(), stubRunner{}, nil)
	m.reasoningEffort = ""
	_ = m.onEvent(events.Event{Type: "reasoning.changed", Data: map[string]string{"effort": "medium"}})
	if strings.TrimSpace(m.reasoningEffort) != "medium" {
		t.Fatalf("expected reasoningEffort=medium, got %q", m.reasoningEffort)
	}
}
