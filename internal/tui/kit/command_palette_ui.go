package kit

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommandInputState captures the first-token slash command analysis for palette handling.
type CommandInputState struct {
	Raw        string
	Trimmed    string
	FirstToken string
	Rest       string
	HasRest    bool
}

// AnalyzeCommandInput parses the current slash-command input into reusable pieces.
func AnalyzeCommandInput(input string) CommandInputState {
	trimmed := strings.TrimSpace(input)
	fields := strings.Fields(trimmed)
	firstToken := ""
	if len(fields) > 0 {
		firstToken = fields[0]
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, firstToken))
	return CommandInputState{
		Raw:        input,
		Trimmed:    trimmed,
		FirstToken: firstToken,
		Rest:       rest,
		HasRest:    rest != "",
	}
}

// Move adjusts palette selection, optionally wrapping around the list boundaries.
func (p *CommandPalette) Move(delta int, wrap bool) {
	if len(p.Matches) == 0 || delta == 0 {
		return
	}
	if !wrap {
		p.Navigate(delta)
		return
	}
	next := p.Selected + delta
	n := len(p.Matches)
	for next < 0 {
		next += n
	}
	p.Selected = next % n
}

// RenderCommandPalette renders the standard bordered inline slash-command palette.
func RenderCommandPalette(contentW int, items []Item, selected, maxDisplay int) string {
	if len(items) == 0 {
		return ""
	}
	if contentW < 1 {
		contentW = 20
	}
	if maxDisplay <= 0 {
		maxDisplay = 6
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff")).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#c0c0c0"))
	opts := SelectorOptions{
		Width:         contentW,
		MaxHeight:     maxDisplay,
		SelectedIndex: selected,
		ShowPrefix:    true,
		Styles: SelectorStyles{
			SelectedTitle:   CloneStyle(selectedStyle),
			UnselectedTitle: CloneStyle(unselectedStyle),
		},
	}
	content := RenderSelector(items, opts)
	return lipgloss.NewStyle().
		Width(contentW).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#404040")).
		Render(content)
}

// PaletteKeyAction is the normalized palette navigation action for a key press.
type PaletteKeyAction int

const (
	PaletteKeyNone PaletteKeyAction = iota
	PaletteKeyAutocomplete
	PaletteKeyMoveUp
	PaletteKeyMoveDown
	PaletteKeyAccept
	PaletteKeyClose
)

// PaletteActionFromKey maps common palette keys to normalized actions.
func PaletteActionFromKey(msg tea.KeyMsg) PaletteKeyAction {
	switch msg.String() {
	case "tab", "right":
		return PaletteKeyAutocomplete
	case "up", "ctrl+p":
		return PaletteKeyMoveUp
	case "down", "ctrl+n":
		return PaletteKeyMoveDown
	case "enter":
		return PaletteKeyAccept
	case "esc", "escape":
		return PaletteKeyClose
	default:
		return PaletteKeyNone
	}
}
