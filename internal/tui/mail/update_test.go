package mail

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleKey_SpaceTogglesBatchExpansion(t *testing.T) {
	m := &Model{
		focus: panelOutbox,
		outbox: []taskEntry{{
			ID:       "callback-batch-1",
			Children: []taskEntry{{ID: "callback-child-1"}, {ID: "callback-child-2"}},
		}},
		expandedByID: map[string]bool{},
	}

	_, _ = m.handleMailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.outbox[0].Expanded {
		t.Fatalf("expected batch task to expand on space")
	}
	if !m.expandedByID["callback-batch-1"] {
		t.Fatalf("expected expansion state persisted by task ID")
	}

	_, _ = m.handleMailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.outbox[0].Expanded {
		t.Fatalf("expected batch task to collapse on second space")
	}
}

func TestHandleKey_SpaceDoesNotToggleWithoutChildren(t *testing.T) {
	m := &Model{
		focus: panelOutbox,
		outbox: []taskEntry{{
			ID: "task-normal",
		}},
		expandedByID: map[string]bool{},
	}

	_, _ = m.handleMailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.outbox[0].Expanded {
		t.Fatalf("did not expect expansion for non-group task")
	}
}
