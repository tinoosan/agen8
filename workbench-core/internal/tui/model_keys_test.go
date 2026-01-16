package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/internal/events"
)

type stubRunner struct {
	final string
	err   error
}

func (s stubRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = ctx
	_ = userMsg
	return s.final, s.err
}

func TestKeyHandling_EnterSubmitsEvenWhenDetailsVisible(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.layout()

	m.single.SetValue("hello")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if cmd == nil {
		t.Fatalf("expected submit cmd, got nil")
	}
	if !updated.turnInFlight {
		t.Fatalf("expected turnInFlight true")
	}
}

func TestKeyHandling_TypingEIsNotHijackedByDetails(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.layout()

	// Simulate typing "e" into the input.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated := m2.(Model)
	if updated.single.Value() != "e" {
		t.Fatalf("expected input to contain %q, got %q", "e", updated.single.Value())
	}
}
