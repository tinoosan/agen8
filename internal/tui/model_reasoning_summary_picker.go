package tui

import (
	"strings"

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
	m.reasoningSummaryPickerOpen = true
	m.reasoningEffortPickerOpen = false
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	sel := 1 // default to "auto"
	cur := strings.ToLower(strings.TrimSpace(m.reasoningSummary))
	for i, opt := range reasoningSummaryOptions {
		if cur != "" && cur == string(opt) {
			sel = i
			break
		}
	}
	m.reasoningSummaryPickerSelected = sel
	m.layout()
}

func (m *Model) closeReasoningSummaryPicker() {
	m.reasoningSummaryPickerOpen = false
	m.reasoningSummaryPickerSelected = 0
	m.layout()
}

func (m *Model) selectReasoningSummaryFromPicker() tea.Cmd {
	if !m.reasoningSummaryPickerOpen {
		return nil
	}
	i := m.reasoningSummaryPickerSelected
	if i < 0 || i >= len(reasoningSummaryOptions) {
		i = 0
	}
	val := string(reasoningSummaryOptions[i])

	m.reasoningSummary = val
	m.reasoningSummaryPickerOpen = false
	m.reasoningSummaryPickerSelected = 0
	m.layout()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/reasoning summary "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}
