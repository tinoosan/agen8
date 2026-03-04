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
	m.reasoningEffortPicker.Options = monitorReasoningEffortOptions
	m.reasoningEffortPicker.OpenAt(m.reasoningEffort, 3) // default: medium
}

// closeReasoningEffortPicker closes the reasoning effort picker.
func (m *monitorModel) closeReasoningEffortPicker() {
	m.reasoningEffortPicker.Close()
}

// updateReasoningEffortPicker handles input when the effort picker is open.
func (m *monitorModel) updateReasoningEffortPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	handled, confirmed := m.reasoningEffortPicker.HandleKey(msg.String())
	if confirmed {
		return m, m.selectReasoningEffort()
	}
	_ = handled
	return m, nil
}

// selectReasoningEffort selects the current option and writes control file.
func (m *monitorModel) selectReasoningEffort() tea.Cmd {
	val := m.reasoningEffortPicker.CurrentValue()
	if val == "" {
		val = monitorReasoningEffortOptions[0]
	}
	m.closeReasoningEffortPicker()
	m.reasoningEffort = val
	return m.writeControl("set_reasoning", map[string]any{"effort": val})
}

func (m *monitorModel) renderReasoningEffortPicker(base string) string {
	return m.renderOptionsPicker(base, "Reasoning Effort", monitorReasoningEffortOptions, m.reasoningEffortPicker.Selected)
}

// openReasoningSummaryPicker opens the reasoning summary picker.
func (m *monitorModel) openReasoningSummaryPicker() {
	m.closeHelpModal()
	m.closeAllPickers()
	m.reasoningSummaryPicker.Options = monitorReasoningSummaryOptions
	m.reasoningSummaryPicker.OpenAt(m.reasoningSummary, 1) // default: auto
}

// closeReasoningSummaryPicker closes the reasoning summary picker.
func (m *monitorModel) closeReasoningSummaryPicker() {
	m.reasoningSummaryPicker.Close()
}

// updateReasoningSummaryPicker handles input when the summary picker is open.
func (m *monitorModel) updateReasoningSummaryPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	handled, confirmed := m.reasoningSummaryPicker.HandleKey(msg.String())
	if confirmed {
		return m, m.selectReasoningSummary()
	}
	_ = handled
	return m, nil
}

// selectReasoningSummary selects the current option and writes control file.
func (m *monitorModel) selectReasoningSummary() tea.Cmd {
	val := m.reasoningSummaryPicker.CurrentValue()
	if val == "" {
		val = monitorReasoningSummaryOptions[0]
	}
	m.closeReasoningSummaryPicker()
	m.reasoningSummary = val
	return m.writeControl("set_reasoning", map[string]any{"summary": val})
}

func (m *monitorModel) renderReasoningSummaryPicker(base string) string {
	return m.renderOptionsPicker(base, "Reasoning Summary", monitorReasoningSummaryOptions, m.reasoningSummaryPicker.Selected)
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
