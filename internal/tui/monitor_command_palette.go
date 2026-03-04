package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

// updateCommandPalette updates the command palette state based on the current input value.
func (m *monitorModel) updateCommandPalette() {
	inputValue := m.input.Value()
	m.commandPalette.Update(inputValue, monitorAvailableCommands, isExactMonitorCommand)
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *monitorModel) autocompleteCommand() {
	newValue, ok := m.commandPalette.Autocomplete(m.input.Value(), true)
	if !ok {
		return
	}
	m.input.SetValue(newValue)
	m.commandPalette.Reset()
}

// handleCommandPaletteKey processes keyboard events when the command palette is showing.
// Returns (cmd, consumed) where cmd can be non-nil for immediate-invoke commands.
func (m *monitorModel) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.commandPalette.Open {
		return nil, false
	}

	switch msg.String() {
	case "tab", "right":
		m.autocompleteCommand()
		return nil, true
	case "up", "ctrl+p":
		m.commandPalette.Navigate(-1)
		return nil, true
	case "down", "ctrl+n":
		m.commandPalette.Navigate(1)
		return nil, true
	case "enter":
		if len(m.commandPalette.Matches) == 0 {
			return nil, true
		}
		selected := m.commandPalette.Selected
		if selected < 0 || selected >= len(m.commandPalette.Matches) {
			selected = 0
		}
		selectedCmd := m.commandPalette.Matches[selected]

		// Determine whether the user already typed anything beyond the first token.
		inputValue := strings.TrimSpace(m.input.Value())
		fields := strings.Fields(inputValue)
		firstToken := ""
		if len(fields) > 0 {
			firstToken = fields[0]
		}
		rest := strings.TrimSpace(strings.TrimPrefix(inputValue, firstToken))
		hasRest := rest != ""

		// If no args are present and the command is invokable, invoke immediately.
		if !hasRest && monitorCommandInvokesWithoutArgs(selectedCmd) {
			m.input.SetValue("")
			m.commandPalette.Reset()
			return m.handleCommand(selectedCmd), true
		}

		m.autocompleteCommand()
		return nil, true
	case "esc", "escape":
		m.commandPalette.Reset()
		return nil, true
	}
	return nil, false
}

// renderCommandPalette renders the inline command palette if open.
func (m *monitorModel) renderCommandPalette(contentW int) string {
	if !m.commandPalette.Open || len(m.commandPalette.Matches) == 0 {
		return ""
	}

	maxDisplay := 6
	outerW := max(20, contentW)
	paletteW := max(1, outerW-4)

	items := make([]kit.Item, len(m.commandPalette.Matches))
	for i, cmd := range m.commandPalette.Matches {
		items[i] = monitorCommandPaletteItem{command: cmd}
	}

	selected := m.commandPalette.Selected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6bbcff")).
		Bold(true)
	unselectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0"))

	opts := kit.SelectorOptions{
		Width:         paletteW,
		MaxHeight:     maxDisplay,
		SelectedIndex: selected,
		ShowPrefix:    true,
		Styles: kit.SelectorStyles{
			SelectedTitle:   kit.CloneStyle(selectedStyle),
			UnselectedTitle: kit.CloneStyle(unselectedStyle),
		},
	}

	paletteContent := kit.RenderSelector(items, opts)

	paletteStyle := lipgloss.NewStyle().
		Width(paletteW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#404040"))

	return paletteStyle.Render(paletteContent)
}

// monitorCommandPaletteItem implements kit.Item for the command palette.
type monitorCommandPaletteItem struct {
	command string
}

func (c monitorCommandPaletteItem) Title() string {
	return c.command
}

func (c monitorCommandPaletteItem) Description() string {
	return monitorCommandDescription(c.command)
}

func (c monitorCommandPaletteItem) FilterValue() string {
	desc := strings.TrimSpace(c.Description())
	if desc == "" {
		return c.command
	}
	return c.command + " " + desc
}
