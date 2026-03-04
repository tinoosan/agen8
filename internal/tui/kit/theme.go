package kit

import "github.com/charmbracelet/lipgloss"

// Base colors — Tokyo Night palette, confirmed identical across activitytui & coordinator.
var (
	ColorOK       = lipgloss.Color("#98c379")
	ColorErr      = lipgloss.Color("#e06c75")
	ColorPending  = lipgloss.Color("#e5c07b")
	ColorAccent   = lipgloss.Color("#7aa2f7")
	ColorThinking = lipgloss.Color("#9d7fdb") // muted purple for thinking
	ColorPlan     = lipgloss.Color("#56b6c2") // teal for plan updates

	StyleOK       = lipgloss.NewStyle().Foreground(ColorOK)
	StyleErr      = lipgloss.NewStyle().Foreground(ColorErr)
	StylePending  = lipgloss.NewStyle().Foreground(ColorPending)
	StyleAccent   = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleThinking = lipgloss.NewStyle().Foreground(ColorThinking)
	StylePlan     = lipgloss.NewStyle().Foreground(ColorPlan)

	// Pill variants (bold + reversed background).
	StylePillOK    = lipgloss.NewStyle().Bold(true).Foreground(ColorOK).Reverse(true).Padding(0, 1)
	StylePillErr   = lipgloss.NewStyle().Bold(true).Foreground(ColorErr).Reverse(true).Padding(0, 1)
	StylePillWhite = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)

	SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)
