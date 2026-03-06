package tui

import (
	"strings"

	"github.com/tinoosan/agen8/internal/tui/kit"
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

func commandPaletteInvokesWithoutArgs(cmd string) bool {
	switch strings.TrimSpace(cmd) {
	case "/new", "/sessions", "/model", "/reasoning-effort", "/reasoning-summary", "/copy", "/editor", "/plan":
		return true
	default:
		return false
	}
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
func (m *Model) updateCommandPalette() {
	prevOpen := m.commandPalette.Open
	prevVisible := 0
	if prevOpen {
		prevVisible = min(len(m.commandPalette.Matches), 6)
	}

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	changed := m.commandPalette.Update(inputValue, availableCommands, isExactCommand)

	newOpen := m.commandPalette.Open
	newVisible := 0
	if newOpen {
		newVisible = min(len(m.commandPalette.Matches), 6)
	}
	if changed || prevOpen != newOpen || prevVisible != newVisible {
		m.layout()
	}
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *Model) autocompleteCommand() {
	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	newValue, ok := m.commandPalette.Autocomplete(inputValue, false)
	if !ok {
		return
	}

	if m.isMulti {
		m.multiline.SetValue(newValue)
	} else {
		m.single.SetValue(newValue)
	}

	m.commandPalette.Reset()
	m.layout()
}

// renderCommandPalette renders the inline command palette if open (for the chat model).
// contentW is the pre-calculated content width (excluding border+padding).
func (m *Model) renderCommandPalette(contentW int) string {
	if !m.commandPalette.Open || len(m.commandPalette.Matches) == 0 {
		return ""
	}
	if contentW < 1 {
		contentW = 20
	}
	maxDisplay := 6

	items := make([]kit.Item, len(m.commandPalette.Matches))
	for i, cmd := range m.commandPalette.Matches {
		items[i] = commandPaletteItem(cmd)
	}

	selected := m.commandPalette.Selected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}
	return kit.RenderCommandPalette(contentW, items, selected, maxDisplay)
}
