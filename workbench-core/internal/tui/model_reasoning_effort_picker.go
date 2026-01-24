package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type reasoningEffortItem string

func (r reasoningEffortItem) Title() string       { return string(r) }
func (r reasoningEffortItem) Description() string { return "" }
func (r reasoningEffortItem) FilterValue() string { return string(r) }

var reasoningEffortOptions = []string{"none", "minimal", "low", "medium", "high", "xhigh"}

func (m *Model) openReasoningEffortPicker() {
	m.reasoningEffortPickerOpen = true
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	// Preselect current effort if known; otherwise default to "medium".
	sel := 3 // medium
	cur := strings.ToLower(strings.TrimSpace(m.reasoningEffort))
	for i, opt := range reasoningEffortOptions {
		if cur != "" && cur == opt {
			sel = i
			break
		}
	}
	m.reasoningEffortPickerSelected = sel
	m.layout()

	// #region agent log
	logDebug("H2", "model_reasoning_effort_picker.go:openReasoningEffortPicker", "picker-open", map[string]interface{}{
		"single":    m.single.Value(),
		"multiline": m.multiline.Value(),
	})
	// #endregion agent log
}

func (m *Model) closeReasoningEffortPicker() {
	m.reasoningEffortPickerOpen = false
	m.reasoningEffortPickerSelected = 0
	m.layout()
}

func (m *Model) selectReasoningEffortFromPicker() tea.Cmd {
	if !m.reasoningEffortPickerOpen {
		return nil
	}
	i := m.reasoningEffortPickerSelected
	if i < 0 || i >= len(reasoningEffortOptions) {
		i = 0
	}
	val := reasoningEffortOptions[i]

	// Optimistic update so the composer status row updates immediately.
	m.reasoningEffort = val
	m.reasoningEffortPickerOpen = false
	m.reasoningEffortPickerSelected = 0
	// Critical: the picker changes composer height; recompute layout so transcript expands back.
	m.layout()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/reasoning effort "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}
