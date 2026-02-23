package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

const monitorHelpText = `Agen8 Monitor – Keyboard Shortcuts

Navigation:
  Tab / Shift+Tab     Cycle focus between panels
  Ctrl+] / Ctrl+[     Cycle side tabs (Activity | Plan | Tasks | Thoughts)
  Ctrl+Up / Ctrl+Down Focus Activity Feed / Details
  Ctrl+Y              Jump to Thoughts tab
  Ctrl+G              Clear run focus lens (/team)
  Ctrl+E              Open $EDITOR for composer

Type a message and press Ctrl+Enter to queue a task, or use a command (below):
  /task <goal>            Queue a task (explicit)
  /new                    Open new-session wizard
  /new [goal]             Start single-agent session
  /new multi-agent <profile> [goal] Start multi-agent session and switch to multi-agent monitor
  /team                   Focus agent in multi-agent session (or clear focus)
  /sessions               Switch session (reattach to its current run)
  /agents                 Switch agent/run in the active session
  /rename <title> Rename current session
  /pause                 Pause run(s) in current session
  /resume                Resume paused run(s) in current session
  /stop                  Hard-stop run(s) and leave them paused
  /clear                 Clear persisted history/events for current context
  /model                 Open model picker
  /model <id>            Set model directly
  /copy                  Copy Agent Output transcript
  /editor                Open $EDITOR to compose (loads back into composer)
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
  d                      Delete selected session (session picker)
  Ctrl+N/P or PgDn/PgUp   Next/prev page (session picker)
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
	maxModalW := max(1, m.width-8)
	if modalWidth > maxModalW {
		modalWidth = maxModalW
	}
	minModalW := min(50, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	modalHeight := len(lines) + 4 // padding + border
	maxModalH := max(1, m.height-6)
	if modalHeight > maxModalH {
		modalHeight = maxModalH
	}
	minModalH := min(15, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
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
