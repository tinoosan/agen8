package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) openHelpModal() {
	m.helpModalOpen = true
	m.helpModalText = m.helpModalContent()
	m.helpModalLines = 0
	if m.helpModalText != "" {
		m.helpModalLines = len(strings.Split(m.helpModalText, "\n"))
	}
	m.helpViewport.SetContent(m.helpModalText)
	m.helpViewport.SetYOffset(0)

	// Ensure viewport has a real size immediately so scrolling clamps correctly.
	contentW, _, vpH := m.helpModalDims()
	m.helpViewport.Width = contentW
	m.helpViewport.Height = vpH
	m.helpViewport.SetYOffset(0)
	m.layout()
}

func (m *Model) closeHelpModal() {
	m.helpModalOpen = false
	m.helpViewport.SetContent("")
	m.helpViewport.SetYOffset(0)
	m.helpModalText = ""
	m.helpModalLines = 0
	m.layout()
}

func (m *Model) helpModalDims() (contentW, contentH, vpH int) {
	outerW := 84
	if outerW > m.width-8 {
		outerW = m.width - 8
	}
	if outerW < 44 {
		outerW = 44
	}
	outerH := 22
	if outerH > m.height-8 {
		outerH = m.height - 8
	}
	if outerH < 12 {
		outerH = 12
	}

	// Keep TOTAL modal size within outerW/outerH (account for padding + border).
	// totalW = contentW + paddingLR(4) + borderLR(2) = contentW + 6
	// totalH = contentH + paddingTB(2) + borderTB(2) = contentH + 4
	contentW = max(10, outerW-6)
	contentH = max(6, outerH-4)

	// Reserve some space for title + footer inside the content area.
	vpH = max(1, contentH-3)
	return contentW, contentH, vpH
}

func (m *Model) clampHelpViewport() {
	if !m.helpModalOpen {
		return
	}
	h := m.helpViewport.Height
	if h < 1 {
		h = 1
	}
	maxY := 0
	if m.helpModalLines > h {
		maxY = m.helpModalLines - h
	}
	if m.helpViewport.YOffset < 0 {
		m.helpViewport.SetYOffset(0)
		return
	}
	if m.helpViewport.YOffset > maxY {
		m.helpViewport.SetYOffset(maxY)
	}
}

func (m *Model) helpModalContent() string {
	// Keep this plain-text (no selection/highlight), and long enough to scroll.
	lines := []string{
		"Shortcuts",
		"",
		"  ctrl+p  help (this screen)",
		"  ctrl+a  toggle activity panel",
		"  tab     cycle focus (input/activity)",
		"  pgup/pgdn  scroll transcript",
		"  ctrl+u/ctrl+f  half-page scroll transcript",
		"  ctrl+y  toggle thinking summary",
		"  ctrl+g  toggle multiline input",
		"  enter   send (single-line)",
		"  ctrl+o  send (multiline)",
		"  esc     close modal/panels",
		"",
		"Copying",
		"",
		"  /copy   copy full transcript",
		"",
		"Composer",
		"",
		"  /editor + Enter  open $EDITOR to compose a message (loads back into input)",
		"  ctrl+e           open $EDITOR with current input prefilled",
		"",
		"Slash commands",
		"",
		"  /model           open model picker",
		"  /model <id>      set model",
		"  /open <path>     open a file via OS",
		"  /editor <path>   edit a workdir file in $EDITOR",
		"  /cd <path>       change workdir",
		"  /pwd             show workdir",
		"  /workdir         alias for /pwd",
		"",
		"Command palette",
		"",
		"  Type '/' in input to show commands; Up/Down to select; Enter to autocomplete; Esc to close.",
		"",
		"References",
		"",
		"  Type '@' to open file picker; Enter inserts an @ref into input; Esc closes.",
		"",
		"Tips",
		"",
		"  - This modal is scrollable: use Up/Down, PgUp/PgDn, or mouse wheel.",
		"  - Press Esc to close.",
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHelpModal(base string) string {
	_ = base
	contentW, contentH, vpH := m.helpModalDims()
	m.helpViewport.Width = contentW
	m.helpViewport.Height = vpH

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eaeaea")).Render("Shortcuts & commands")
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Render("↑/↓ scroll  pgup/pgdn faster  esc close")
	body := m.helpViewport.View()
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)

	modalStyle := lipgloss.NewStyle().
		Width(contentW).
		Height(contentH).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Background(lipgloss.Color("#1a1a1a")).
		Foreground(lipgloss.Color("#eaeaea"))

	modalContent := modalStyle.Render(content)
	modalLines := strings.Split(modalContent, "\n")
	modalHeightActual := len(modalLines)
	modalWidthActual := 0
	for _, line := range modalLines {
		if w := lipgloss.Width(line); w > modalWidthActual {
			modalWidthActual = w
		}
	}

	topPos := (m.height - modalHeightActual) / 2
	if topPos < 0 {
		topPos = 0
	}
	leftPos := (m.width - modalWidthActual) / 2
	if leftPos < 0 {
		leftPos = 0
	}

	// Render over a blank backdrop to avoid ANSI corruption artifacts.
	result := make([]string, m.height)
	for i := 0; i < m.height; i++ {
		result[i] = strings.Repeat(" ", max(1, m.width))
		if i >= topPos && i < topPos+modalHeightActual {
			lineIdx := i - topPos
			if lineIdx < len(modalLines) {
				result[i] = strings.Repeat(" ", leftPos) + modalLines[lineIdx]
			}
		}
	}
	return strings.Join(result, "\n")
}
