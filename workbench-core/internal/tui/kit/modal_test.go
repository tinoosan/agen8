package kit

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderOverlay_ClampsLinesToScreenWidth(t *testing.T) {
	opts := ModalOptions{
		Content:      "This is a very long content line that should be clipped to screen width.",
		ScreenWidth:  20,
		ScreenHeight: 8,
		Width:        18,
		Height:       4,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}

	got := RenderOverlay(opts)
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if w := lipgloss.Width(line); w > opts.ScreenWidth {
			t.Fatalf("line %d width %d exceeds screen width %d", i, w, opts.ScreenWidth)
		}
	}
}

func TestRenderOverlay_TinyScreenClampsWidth(t *testing.T) {
	opts := ModalOptions{
		Content:      "12345678901234567890",
		ScreenWidth:  10,
		ScreenHeight: 5,
		Width:        10,
		Height:       3,
	}

	got := RenderOverlay(opts)
	lines := strings.Split(got, "\n")
	if len(lines) != opts.ScreenHeight {
		t.Fatalf("expected %d lines, got %d", opts.ScreenHeight, len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > opts.ScreenWidth {
			t.Fatalf("line %d width %d exceeds screen width %d", i, w, opts.ScreenWidth)
		}
	}
}
