package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

const monitorHelpText = `Workbench Monitor – Keyboard Shortcuts

Navigation:
  Tab / Shift+Tab     Cycle focus between panels
  Ctrl+] / Ctrl+[     Cycle side tabs (Activity | Plan | Tasks | Thoughts)
  Ctrl+Up / Ctrl+Down Focus Activity Feed / Details
  Ctrl+Y              Jump to Thoughts tab

Commands (type in composer, submit with Ctrl+Enter):
  /task <goal>           Queue a new task for the agent
  /model                 Open model picker
  /model <id>            Set model directly
  /profile <ref>         Switch agent profile
  /memory search <query> Search vector memory
  /reasoning-effort      Set reasoning effort level
  /reasoning-summary     Set reasoning summary mode
  /help                  Show this help modal
  /quit                  Exit monitor

File References:
  @<path>                Autocomplete file paths in composer

Modal Controls:
  Escape                 Close any open modal
  Enter                  Confirm selection
  ↑/↓ or j/k             Navigate list items
  Type to filter         Filter list items

Press Escape or ? to close this help`

// openHelpModal opens the help modal overlay.
func (m *monitorModel) openHelpModal() {
	m.helpModalOpen = true
}

// closeHelpModal closes the help modal overlay.
func (m *monitorModel) closeHelpModal() {
	m.helpModalOpen = false
}

// renderHelpModal renders the help text as a centered modal overlay.
func (m *monitorModel) renderHelpModal(base string) string {
	// Calculate modal dimensions
	lines := strings.Split(monitorHelpText, "\n")
	maxLineWidth := 0
	for _, line := range lines {
		if len(line) > maxLineWidth {
			maxLineWidth = len(line)
		}
	}

	modalWidth := maxLineWidth + 6 // padding + border
	if modalWidth > m.width-8 {
		modalWidth = m.width - 8
	}
	if modalWidth < 50 {
		modalWidth = 50
	}

	modalHeight := len(lines) + 4 // padding + border
	if modalHeight > m.height-6 {
		modalHeight = m.height - 6
	}
	if modalHeight < 15 {
		modalHeight = 15
	}

	// Style the help text
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#d0d0d0")).
		Width(modalWidth - 4)

	content := contentStyle.Render(monitorHelpText)

	opts := kit.ModalOptions{
		Content:      content,
		ScreenWidth:  m.width,
		ScreenHeight: m.height,
		Width:        modalWidth,
		Height:       modalHeight,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}

	_ = base
	return kit.RenderOverlay(opts)
}
