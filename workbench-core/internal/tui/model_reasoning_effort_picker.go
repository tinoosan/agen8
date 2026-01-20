package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var reasoningEffortOptions = []string{"none", "minimal", "low", "medium", "high", "xhigh"}

func (m *Model) openReasoningEffortPicker() {
	// #region agent log
	// Hypothesis H1/H3: picker close causes base view to render < terminal height.
	debugLog("debug-session", "pre-fix", "H3", "model_reasoning_effort_picker.go:11", "openReasoningEffortPicker", map[string]interface{}{
		"termH":       m.height,
		"termW":       m.width,
		"showDetails": m.showDetails,
		"effort":      strings.TrimSpace(m.reasoningEffort),
	})
	// #endregion

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
	vh := lipgloss.Height(m.View())
	debugLog("debug-session", "pre-fix", "H3", "model_reasoning_effort_picker.go:28", "openReasoningEffortPicker after layout", map[string]interface{}{
		"pickerSelected": sel,
		"viewH":          vh,
		"termH":          m.height,
		"delta":          m.height - vh,
	})
	// #endregion
}

func (m *Model) closeReasoningEffortPicker() {
	// #region agent log
	debugLog("debug-session", "pre-fix", "H1", "model_reasoning_effort_picker.go:30", "closeReasoningEffortPicker start", map[string]interface{}{
		"termH":          m.height,
		"termW":          m.width,
		"wasOpen":        m.reasoningEffortPickerOpen,
		"pickerSelected": m.reasoningEffortPickerSelected,
	})
	// #endregion

	m.reasoningEffortPickerOpen = false
	m.reasoningEffortPickerSelected = 0
	m.layout()

	// #region agent log
	vh := lipgloss.Height(m.View())
	debugLog("debug-session", "pre-fix", "H1", "model_reasoning_effort_picker.go:34", "closeReasoningEffortPicker after layout", map[string]interface{}{
		"viewH": vh,
		"termH": m.height,
		"delta": m.height - vh,
	})
	// #endregion
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

	// Measure before close/layout so we can confirm sizing changes.
	// #region agent log
	viewHBefore := lipgloss.Height(m.View())
	// #endregion

	// Optimistic update so the composer status row updates immediately.
	m.reasoningEffort = val
	m.reasoningEffortPickerOpen = false
	m.reasoningEffortPickerSelected = 0
	// Critical: the picker changes composer height; recompute layout so transcript expands back.
	m.layout()

	// #region agent log
	// Hypothesis H2: selection path closes without re-layout, leaving stale rows.
	vh := lipgloss.Height(m.View())
	debugLog("debug-session", "pre-fix", "H2", "model_reasoning_effort_picker.go:36", "selectReasoningEffortFromPicker", map[string]interface{}{
		"val":   val,
		"viewHBefore": viewHBefore,
		"viewHAfter":  vh,
		"termH":       m.height,
		"deltaBefore": m.height - viewHBefore,
		"deltaAfter":  m.height - vh,
	})
	// #endregion

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/reasoning effort "+val)
		return turnDoneMsg{final: final, err: err}
	}
}
