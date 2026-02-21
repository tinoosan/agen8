package kit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type ModalOptions struct {
	Content      string
	ScreenWidth  int
	ScreenHeight int
	Width        int
	Height       int
	Padding      [2]int // [vertical, horizontal]
	BorderStyle  lipgloss.Border
	BorderColor  lipgloss.Color
	Foreground   lipgloss.Color
}

func RenderOverlay(opts ModalOptions) string {
	if opts.ScreenWidth <= 0 || opts.ScreenHeight <= 0 {
		return opts.Content
	}

	modalWidth := opts.Width
	if modalWidth <= 0 {
		modalWidth = lipgloss.Width(opts.Content)
	}
	if modalWidth <= 0 {
		modalWidth = opts.ScreenWidth
	}
	modalWidth = clampInt(modalWidth, 1, opts.ScreenWidth)

	modalHeight := opts.Height
	if modalHeight <= 0 {
		modalHeight = lipgloss.Height(opts.Content)
	}
	if modalHeight <= 0 {
		modalHeight = opts.ScreenHeight
	}
	modalHeight = clampInt(modalHeight, 1, opts.ScreenHeight)

	paddingY := opts.Padding[0]
	paddingX := opts.Padding[1]
	if paddingY == 0 {
		paddingY = 1
	}
	if paddingX == 0 {
		paddingX = 2
	}

	border := opts.BorderStyle
	if border == (lipgloss.Border{}) {
		border = lipgloss.RoundedBorder()
	}

	borderColor := opts.BorderColor
	if borderColor == "" {
		borderColor = BorderColorDefault
	}

	foreground := opts.Foreground
	if foreground == "" {
		foreground = lipgloss.Color("#eaeaea")
	}

	style := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		Padding(paddingY, paddingX).
		BorderStyle(border).
		BorderForeground(borderColor).
		Foreground(foreground)

	content := style.Render(opts.Content)

	lines := strings.Split(content, "\n")
	linesHeight := len(lines)
	maxLineWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxLineWidth {
			maxLineWidth = w
		}
	}

	top := (opts.ScreenHeight - linesHeight) / 2
	if top < 0 {
		top = 0
	}
	left := (opts.ScreenWidth - maxLineWidth) / 2
	if left < 0 {
		left = 0
	}

	canvas := make([]string, maxInt(1, opts.ScreenHeight))
	for i := 0; i < len(canvas); i++ {
		canvas[i] = strings.Repeat(" ", maxInt(1, opts.ScreenWidth))
		if i >= top && i < top+linesHeight {
			lineIdx := i - top
			if lineIdx < len(lines) {
				line := lines[lineIdx]
				composed := strings.Repeat(" ", left) + line
				if lipgloss.Width(composed) > opts.ScreenWidth {
					composed = lipgloss.NewStyle().MaxWidth(opts.ScreenWidth).Render(composed)
				}
				canvas[i] = composed
			}
		}
	}

	return strings.Join(canvas, "\n")
}
