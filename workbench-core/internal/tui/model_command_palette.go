package tui

import (
	"strings"
)

type commandPaletteItem string

func (c commandPaletteItem) Title() string       { return string(c) }
func (c commandPaletteItem) Description() string { return "" }
func (c commandPaletteItem) FilterValue() string { return string(c) }

// Hardcoded list of available slash commands for the command palette.
var availableCommands = []string{
	"/new",
	"/sessions",
	"/model",
	"/reasoning-effort",
	"/reasoning-summary",
	"/copy",
	"/editor",
	"/cd",
	"/plan",
}

func isExactCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, cmd := range availableCommands {
		if s == cmd {
			return true
		}
	}
	return false
}

// updateCommandPalette updates the command palette state based on the current input value.
// It detects if the input starts with "/" and filters commands accordingly.
func (m *Model) updateCommandPalette() {
	prevOpen := m.commandPaletteOpen
	prevVisible := 0
	if prevOpen {
		prevVisible = min(len(m.commandPaletteMatches), 6)
	}

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

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
		if isExactCommand(firstToken) && strings.ContainsAny(inputValue, " \t\n") {
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
			if prevOpen {
				m.layout()
			}
			return
		}

		// Filter commands that match the typed prefix.
		matches := []string{}
		for _, cmd := range availableCommands {
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

	// If the palette visibility/height changed, recompute layout so total View height
	// stays within the terminal bounds (avoids header/transcript clipping).
	newOpen := m.commandPaletteOpen
	newVisible := 0
	if newOpen {
		newVisible = min(len(m.commandPaletteMatches), 6)
	}
	if prevOpen != newOpen || prevVisible != newVisible {
		m.layout()
	}
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *Model) autocompleteCommand() {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return
	}
	if m.commandPaletteSelected < 0 || m.commandPaletteSelected >= len(m.commandPaletteMatches) {
		return
	}

	selectedCmd := m.commandPaletteMatches[m.commandPaletteSelected]

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	// Extract the first token and any trailing args.
	fields := strings.Fields(inputValue)
	if len(fields) == 0 {
		// Empty input, just set the command.
		if m.isMulti {
			m.multiline.SetValue(selectedCmd)
		} else {
			m.single.SetValue(selectedCmd)
		}
	} else {
		// Replace first token with selected command, preserve rest.
		rest := strings.TrimSpace(strings.TrimPrefix(inputValue, fields[0]))
		newValue := selectedCmd
		if rest != "" {
			newValue = selectedCmd + " " + rest
		}
		if m.isMulti {
			m.multiline.SetValue(newValue)
		} else {
			m.single.SetValue(newValue)
		}
	}

	// Close palette after autocomplete.
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	// Recompute layout so the transcript area expands again.
	m.layout()
}
