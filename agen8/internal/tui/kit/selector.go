package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Item interface {
	Title() string
	Description() string
	FilterValue() string
}

type SelectorStyles struct {
	SelectedTitle   *lipgloss.Style
	SelectedDesc    *lipgloss.Style
	UnselectedTitle *lipgloss.Style
	UnselectedDesc  *lipgloss.Style
}

type SelectorOptions struct {
	Width         int
	MaxHeight     int
	SelectedIndex int
	ShowPrefix    bool
	Spacing       int
	Styles        SelectorStyles
}

func RenderSelector(items []Item, opts SelectorOptions) string {
	if opts.Width <= 0 || len(items) == 0 {
		return ""
	}

	styles := opts.Styles.withDefaults()

	selected := opts.SelectedIndex
	if selected < 0 {
		selected = 0
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}

	scrollOffset := 0
	visibleItems := items
	selectedInView := selected
	if opts.MaxHeight > 0 && len(items) > opts.MaxHeight {
		maxDisplay := opts.MaxHeight
		start := selected - maxDisplay/2
		if start < 0 {
			start = 0
		}
		if start+maxDisplay > len(items) {
			start = len(items) - maxDisplay
		}
		scrollOffset = start
		visibleItems = items[start : start+maxDisplay]
		selectedInView = selected - scrollOffset
	}

	selectedPrefix := "› "
	unselectedPrefix := "  "
	if !opts.ShowPrefix {
		selectedPrefix = ""
		unselectedPrefix = ""
	}

	lines := make([]string, 0, len(visibleItems))
	for idx, item := range visibleItems {
		prefix := unselectedPrefix
		titleStyle := styles.UnselectedTitle
		descStyle := styles.UnselectedDesc
		if idx == selectedInView {
			prefix = selectedPrefix
			titleStyle = styles.SelectedTitle
			descStyle = styles.SelectedDesc
		}

		availableW := maxInt(1, opts.Width-lipgloss.Width(prefix))
		title := prefix + TruncateRight(item.Title(), availableW)
		lines = append(lines, titleStyle.Render(title))

		desc := strings.TrimSpace(item.Description())
		if desc != "" {
			descWidth := maxInt(1, opts.Width-int(lipgloss.Width(prefix)))
			descLine := strings.Repeat(" ", lipgloss.Width(prefix)) + TruncateRight(desc, descWidth)
			lines = append(lines, descStyle.Render(descLine))
		}

		if opts.Spacing > 0 && idx < len(visibleItems)-1 {
			for i := 0; i < opts.Spacing; i++ {
				lines = append(lines, "")
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (s SelectorStyles) withDefaults() SelectorStyles {
	def := defaultSelectorStyles()
	if s.SelectedTitle == nil {
		s.SelectedTitle = def.SelectedTitle
	}
	if s.SelectedDesc == nil {
		s.SelectedDesc = def.SelectedDesc
	}
	if s.UnselectedTitle == nil {
		s.UnselectedTitle = def.UnselectedTitle
	}
	if s.UnselectedDesc == nil {
		s.UnselectedDesc = def.UnselectedDesc
	}
	return s
}

func defaultSelectorStyles() SelectorStyles {
	return SelectorStyles{
		SelectedTitle:   CloneStyle(StyleSelectorSelectedTitle),
		SelectedDesc:    CloneStyle(StyleSelectorSelectedDesc),
		UnselectedTitle: CloneStyle(StyleSelectorUnselectedTitle),
		UnselectedDesc:  CloneStyle(StyleSelectorUnselectedDesc),
	}
}
