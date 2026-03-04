package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type reasoningEffortItem string

func (r reasoningEffortItem) Title() string       { return string(r) }
func (r reasoningEffortItem) Description() string { return "" }
func (r reasoningEffortItem) FilterValue() string { return string(r) }

var reasoningEffortOptions = []string{"none", "minimal", "low", "medium", "high", "xhigh"}

func (m *Model) openReasoningEffortPicker() {
	if m.runtimeChangeLocked("changing reasoning effort") {
		return
	}
	m.reasoningEffortPicker.Options = reasoningEffortOptions
	m.reasoningEffortPicker.OpenAt(m.reasoningEffort, 3) // default: medium (index 3)
	m.commandPalette.Reset()
	m.layout()
}

func (m *Model) closeReasoningEffortPicker() {
	m.reasoningEffortPicker.Close()
	m.layout()
}

func (m *Model) selectReasoningEffortFromPicker() tea.Cmd {
	if !m.reasoningEffortPicker.Open {
		return nil
	}
	val := m.reasoningEffortPicker.CurrentValue()
	if val == "" {
		val = reasoningEffortOptions[0]
	}

	// Optimistic update so the composer status row updates immediately.
	m.reasoningEffort = val
	m.reasoningEffortPicker.Close()
	// Critical: the picker changes composer height; recompute layout so transcript expands back.
	m.layout()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/reasoning effort "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}
