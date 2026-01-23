package kit

import "github.com/charmbracelet/lipgloss"

var (
	StyleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	StyleBold = lipgloss.NewStyle().Bold(true)

	StyleSelectorSelectedTitle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#eaeaea")).
					Bold(true)
	StyleSelectorSelectedDesc = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#9ad0ff"))
	StyleSelectorUnselectedTitle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#b0b0b0"))
	StyleSelectorUnselectedDesc = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#b0b0b0"))

	StyleStatusKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true)
	StyleStatusValue = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#eaeaea"))

	BorderColorDefault = lipgloss.Color("#404040")
	BorderColorAccent  = lipgloss.Color("#6bbcff")
)
