package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// OptionsPicker manages the state of a small inline options-selection widget.
type OptionsPicker struct {
	Open     bool
	Options  []string
	Selected int
}

// OpenAt opens the picker and pre-selects the option matching currentValue.
// defaultIdx is used when currentValue is blank or unrecognised.
func (p *OptionsPicker) OpenAt(currentValue string, defaultIdx int) {
	p.Open = true
	p.Selected = defaultIdx
	cur := strings.ToLower(strings.TrimSpace(currentValue))
	for i, opt := range p.Options {
		if strings.EqualFold(opt, cur) {
			p.Selected = i
			break
		}
	}
	p.Selected = clampInt(p.Selected, 0, max(0, len(p.Options)-1))
}

// Close closes the picker without confirming.
func (p *OptionsPicker) Close() { p.Open = false }

// Navigate moves the selection by delta, clamped to valid range.
func (p *OptionsPicker) Navigate(delta int) {
	p.Selected = clampInt(p.Selected+delta, 0, max(0, len(p.Options)-1))
}

// CurrentValue returns the currently selected option string, or "" if empty.
func (p *OptionsPicker) CurrentValue() string {
	if len(p.Options) == 0 || p.Selected < 0 || p.Selected >= len(p.Options) {
		return ""
	}
	return p.Options[p.Selected]
}

// HandleKey processes a key name and updates state.
// Returns (handled, confirmed): handled=true means the key was consumed;
// confirmed=true means the user pressed Enter to accept the selection.
func (p *OptionsPicker) HandleKey(key string) (handled, confirmed bool) {
	switch key {
	case "up", "k":
		p.Navigate(-1)
		return true, false
	case "down", "j":
		p.Navigate(1)
		return true, false
	case "esc", "escape":
		p.Close()
		return true, false
	case "enter":
		p.Open = false
		return true, true
	}
	return false, false
}

// Render draws the picker as a string suitable for embedding in a modal.
func (p *OptionsPicker) Render(title string, contentW int) string {
	if contentW < 1 {
		contentW = 20
	}
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Bold(true)
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0"))
	selStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")).Bold(true)
	var sb strings.Builder
	if title != "" {
		sb.WriteString(titleStyle.Render(title) + "\n\n")
	}
	for i, opt := range p.Options {
		if i == p.Selected {
			sb.WriteString(selStyle.Render("› " + opt))
		} else {
			sb.WriteString(rowStyle.Render("  " + opt))
		}
		if i < len(p.Options)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n\n" + StyleDim.Render("↑/↓ to select, Enter to confirm, Esc to cancel"))
	return sb.String()
}
