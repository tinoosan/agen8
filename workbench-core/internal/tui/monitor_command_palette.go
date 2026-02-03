package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

// Hardcoded list of available slash commands for the monitor command palette.
var monitorAvailableCommands = []string{
	"/task",
	"/model",
	"/profile",
	"/reasoning-effort",
	"/reasoning-summary",
	"/memory search",
	"/help",
	"/quit",
}

func isExactMonitorCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, cmd := range monitorAvailableCommands {
		if s == cmd {
			return true
		}
	}
	return false
}

// updateCommandPalette updates the command palette state based on the current input value.
// It detects if the input starts with "/" and filters commands accordingly.
func (m *monitorModel) updateCommandPalette() {
	inputValue := m.input.Value()

	// Extract the first token (command part) from the input.
	fields := strings.Fields(inputValue)
	var firstToken string
	if len(fields) > 0 {
		firstToken = fields[0]
	} else {
		// Empty input or only whitespace - use the raw value.
		firstToken = strings.TrimSpace(inputValue)
	}

	// Check if we're in command mode (starts with "/").
	if strings.HasPrefix(firstToken, "/") {
		// If the user has already completed a valid command token and is now typing
		// arguments (i.e. there is whitespace after the first token), keep the palette closed.
		if isExactMonitorCommand(firstToken) && strings.ContainsAny(inputValue, " \t\n") {
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
			return
		}

		// Filter commands that match the typed prefix.
		matches := []string{}
		for _, cmd := range monitorAvailableCommands {
			if strings.HasPrefix(cmd, firstToken) {
				matches = append(matches, cmd)
			}
		}

		if len(matches) > 0 {
			m.commandPaletteOpen = true
			m.commandPaletteMatches = matches
			// Ensure selected index is valid.
			if m.commandPaletteSelected >= len(matches) {
				m.commandPaletteSelected = 0
			}
			if m.commandPaletteSelected < 0 {
				m.commandPaletteSelected = 0
			}
		} else {
			// No matches, close palette.
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
		}
	} else {
		// Not in command mode, close palette.
		m.commandPaletteOpen = false
		m.commandPaletteMatches = nil
		m.commandPaletteSelected = 0
	}
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *monitorModel) autocompleteCommand() {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return
	}
	if m.commandPaletteSelected < 0 || m.commandPaletteSelected >= len(m.commandPaletteMatches) {
		return
	}

	selectedCmd := m.commandPaletteMatches[m.commandPaletteSelected]
	inputValue := m.input.Value()

	// Extract the first token and any trailing args.
	fields := strings.Fields(inputValue)
	if len(fields) == 0 {
		// Empty input, just set the command.
		m.input.SetValue(selectedCmd + " ")
	} else {
		// Replace first token with selected command, preserve rest.
		rest := strings.TrimSpace(strings.TrimPrefix(inputValue, fields[0]))
		newValue := selectedCmd
		if rest != "" {
			newValue = selectedCmd + " " + rest
		} else {
			newValue = selectedCmd + " "
		}
		m.input.SetValue(newValue)
	}

	// Close palette after autocomplete.
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0
}

// handleCommandPaletteKey processes keyboard events when the command palette is showing.
// Returns true if the key was consumed, false otherwise.
func (m *monitorModel) handleCommandPaletteKey(msg tea.KeyMsg) bool {
	if !m.commandPaletteOpen {
		return false
	}

	switch msg.String() {
	case "tab", "right":
		m.autocompleteCommand()
		return true
	case "up", "ctrl+p":
		if m.commandPaletteSelected > 0 {
			m.commandPaletteSelected--
		}
		return true
	case "down", "ctrl+n":
		if m.commandPaletteSelected < len(m.commandPaletteMatches)-1 {
			m.commandPaletteSelected++
		}
		return true
	case "esc", "escape":
		m.commandPaletteOpen = false
		m.commandPaletteMatches = nil
		m.commandPaletteSelected = 0
		return true
	}
	return false
}

// renderCommandPalette renders the inline command palette if open.
func (m *monitorModel) renderCommandPalette() string {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return ""
	}

	maxDisplay := 6
	outerW := max(20, m.width-8)
	contentW := max(1, outerW-4)

	items := make([]kit.Item, len(m.commandPaletteMatches))
	for i, cmd := range m.commandPaletteMatches {
		items[i] = monitorCommandPaletteItem(cmd)
	}

	selected := m.commandPaletteSelected
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
		Width:         contentW,
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
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#404040"))

	return paletteStyle.Render(paletteContent)
}

// monitorCommandPaletteItem implements kit.Item for the command palette.
type monitorCommandPaletteItem string

func (c monitorCommandPaletteItem) Title() string       { return string(c) }
func (c monitorCommandPaletteItem) Description() string { return "" }
func (c monitorCommandPaletteItem) FilterValue() string { return string(c) }
