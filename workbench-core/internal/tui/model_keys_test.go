package tui

import (
	"context"
	"strings"
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

func TestTranscript_FirstUserMessageVisibleAtTop(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Simulate first user message.
	m.addTranscriptItem(transcriptItem{kind: transcriptUser, text: "line1\nline2\nline3"})

	if m.transcript.YOffset != 0 {
		t.Fatalf("expected first message to start at top (YOffset=0), got %d", m.transcript.YOffset)
	}
}

func TestTranscript_ScrollAnchorsToTurnStartOnCompletion(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	// Small viewport to force scrolling with a long agent message.
	m.width = 120
	m.height = 20
	m.layout()

	// Start a turn.
	_ = m.submit("User request\nwith multiple lines")

	// Add a long agent message to create lots of lines.
	long := ""
	for i := 0; i < 200; i++ {
		long += "line\n"
	}
	m2, _ := m.Update(turnDoneMsg{final: long})
	updated := m2.(Model)

	if updated.lastTurnUserItemIdx < 0 {
		t.Fatalf("expected lastTurnUserItemIdx to be set")
	}
	if updated.lastTurnUserItemIdx >= len(updated.transcriptItemStartLine) {
		t.Fatalf("expected transcriptItemStartLine to include user item")
	}
	want := updated.transcriptItemStartLine[updated.lastTurnUserItemIdx]
	if updated.transcript.YOffset != want {
		t.Fatalf("expected YOffset to anchor to turn start %d, got %d", want, updated.transcript.YOffset)
	}
}

func TestLayout_WithActivitySidebar_DoesNotClipHeader(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.layout()

	view := m.View()
	// Header should be present at the top of the view string.
	if !strings.HasPrefix(view, "workbench") {
		t.Fatalf("expected header to start with %q, got prefix %q", "workbench", view[:min(len(view), 32)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
