package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type reasoningSummaryItem string

func (r reasoningSummaryItem) Title() string       { return string(r) }
func (r reasoningSummaryItem) Description() string { return "" }
func (r reasoningSummaryItem) FilterValue() string { return string(r) }

var reasoningSummaryOptions = []reasoningSummaryItem{"off", "auto", "concise", "detailed"}

func (m *Model) openReasoningSummaryPicker() {
	if m.runtimeChangeLocked("changing reasoning summary") {
		return
	}
	opts := make([]string, len(reasoningSummaryOptions))
	for i, o := range reasoningSummaryOptions {
		opts[i] = string(o)
	}
	m.reasoningSummaryPicker.Options = opts
	m.reasoningSummaryPicker.OpenAt(m.reasoningSummary, 1) // default: auto (index 1)
	m.reasoningEffortPicker.Close()
	m.commandPalette.Reset()
	m.layout()
}

func (m *Model) closeReasoningSummaryPicker() {
	m.reasoningSummaryPicker.Close()
	m.layout()
}

func (m *Model) selectReasoningSummaryFromPicker() tea.Cmd {
	if !m.reasoningSummaryPicker.Open {
		return nil
	}
	val := m.reasoningSummaryPicker.CurrentValue()
	if val == "" {
		val = string(reasoningSummaryOptions[0])
	}

	m.reasoningSummary = val
	m.reasoningSummaryPicker.Close()
	m.layout()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/reasoning summary "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}
