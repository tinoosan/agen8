package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

var monitorReasoningEffortOptions = []string{"none", "minimal", "low", "medium", "high", "xhigh"}
var monitorReasoningSummaryOptions = []string{"off", "auto", "concise", "detailed"}

// openReasoningEffortPicker opens the reasoning effort picker.
func (m *monitorModel) openReasoningEffortPicker() {
	m.closeHelpModal()
	m.closeAllPickers()
	m.reasoningEffortPickerOpen = true
	sel := 3 // default to medium
	cur := strings.ToLower(strings.TrimSpace(m.reasoningEffort))
	for i, opt := range monitorReasoningEffortOptions {
		if strings.EqualFold(opt, cur) {
			sel = i
			break
		}
	}
	m.reasoningEffortPickerSelected = sel
}

// closeReasoningEffortPicker closes the reasoning effort picker.
func (m *monitorModel) closeReasoningEffortPicker() {
	m.reasoningEffortPickerOpen = false
	m.reasoningEffortPickerSelected = 0
}

// updateReasoningEffortPicker handles input when the effort picker is open.
func (m *monitorModel) updateReasoningEffortPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.closeReasoningEffortPicker()
		return m, nil
	case "enter":
		return m, m.selectReasoningEffort()
	case "up", "k":
		if m.reasoningEffortPickerSelected > 0 {
			m.reasoningEffortPickerSelected--
		}
		return m, nil
	case "down", "j":
		if m.reasoningEffortPickerSelected < len(monitorReasoningEffortOptions)-1 {
			m.reasoningEffortPickerSelected++
		}
		return m, nil
	}
	return m, nil
}

// selectReasoningEffort selects the current option and writes control file.
func (m *monitorModel) selectReasoningEffort() tea.Cmd {
	if m.reasoningEffortPickerSelected < 0 || m.reasoningEffortPickerSelected >= len(monitorReasoningEffortOptions) {
		m.reasoningEffortPickerSelected = 0
	}
	val := monitorReasoningEffortOptions[m.reasoningEffortPickerSelected]
	m.closeReasoningEffortPicker()
	m.reasoningEffort = val
	return m.writeControl("set_reasoning", map[string]any{"effort": val})
}

func (m *monitorModel) renderReasoningEffortPicker(base string) string {
	return m.renderOptionsPicker(base, "Reasoning Effort", monitorReasoningEffortOptions, m.reasoningEffortPickerSelected)
}

// openReasoningSummaryPicker opens the reasoning summary picker.
func (m *monitorModel) openReasoningSummaryPicker() {
	m.closeHelpModal()
	m.closeAllPickers()
	m.reasoningSummaryPickerOpen = true
	sel := 1 // default to auto
	cur := strings.ToLower(strings.TrimSpace(m.reasoningSummary))
	for i, opt := range monitorReasoningSummaryOptions {
		if strings.EqualFold(opt, cur) {
			sel = i
			break
		}
	}
	m.reasoningSummaryPickerSelected = sel
}

// closeReasoningSummaryPicker closes the reasoning summary picker.
func (m *monitorModel) closeReasoningSummaryPicker() {
	m.reasoningSummaryPickerOpen = false
	m.reasoningSummaryPickerSelected = 0
}

// updateReasoningSummaryPicker handles input when the summary picker is open.
func (m *monitorModel) updateReasoningSummaryPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.closeReasoningSummaryPicker()
		return m, nil
	case "enter":
		return m, m.selectReasoningSummary()
	case "up", "k":
		if m.reasoningSummaryPickerSelected > 0 {
			m.reasoningSummaryPickerSelected--
		}
		return m, nil
	case "down", "j":
		if m.reasoningSummaryPickerSelected < len(monitorReasoningSummaryOptions)-1 {
			m.reasoningSummaryPickerSelected++
		}
		return m, nil
	}
	return m, nil
}

// selectReasoningSummary selects the current option and writes control file.
func (m *monitorModel) selectReasoningSummary() tea.Cmd {
	if m.reasoningSummaryPickerSelected < 0 || m.reasoningSummaryPickerSelected >= len(monitorReasoningSummaryOptions) {
		m.reasoningSummaryPickerSelected = 0
	}
	val := monitorReasoningSummaryOptions[m.reasoningSummaryPickerSelected]
	m.closeReasoningSummaryPicker()
	m.reasoningSummary = val
	return m.writeControl("set_reasoning", map[string]any{"summary": val})
}

func (m *monitorModel) renderReasoningSummaryPicker(base string) string {
	return m.renderOptionsPicker(base, "Reasoning Summary", monitorReasoningSummaryOptions, m.reasoningSummaryPickerSelected)
}

// renderOptionsPicker renders a simple options picker modal.
func (m *monitorModel) renderOptionsPicker(base, title string, options []string, selected int) string {
	dims := kit.ComputeModalDims(m.width, m.height, 40, len(options)+6, 30, 8, 6, 0)

	styleTitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	styleRow := lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0"))
	styleSel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#eaeaea")).
		Bold(true)

	var lines []string
	lines = append(lines, styleTitle.Render(title))
	lines = append(lines, "")
	for i, opt := range options {
		prefix := "  "
		style := styleRow
		if i == selected {
			prefix = "› "
			style = styleSel
		}
		lines = append(lines, style.Render(prefix+opt))
	}
	lines = append(lines, "")
	lines = append(lines, kit.StyleDim.Render("↑/↓ to select, Enter to confirm, Esc to cancel"))

	content := strings.Join(lines, "\n")

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
