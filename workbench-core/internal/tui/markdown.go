package tui

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/glamour"
)

// markdownRenderer caches Glamour renderers keyed by wrap width.
//
// Glamour renderers are relatively expensive to construct. The TUI re-renders the transcript
// on resize and when toggling the right pane, so caching keeps the UI responsive.
type markdownRenderer struct {
	mu        sync.Mutex
	byWidth   map[int]*glamour.TermRenderer
	lastWidth int
	last      *glamour.TermRenderer
}

func newMarkdownRenderer() *markdownRenderer {
	return &markdownRenderer{byWidth: map[int]*glamour.TermRenderer{}}
}

func (r *markdownRenderer) render(md string, width int) string {
	if width <= 0 {
		width = 80
	}
	tr, err := r.get(width)
	if err != nil {
		// Fallback: preserve raw markdown if renderer can't be constructed.
		return md
	}
	out, err := tr.Render(md)
	if err != nil {
		return md
	}
	return out
}

func (r *markdownRenderer) get(width int) (*glamour.TermRenderer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.last != nil && r.lastWidth == width {
		return r.last, nil
	}
	if tr := r.byWidth[width]; tr != nil {
		r.lastWidth = width
		r.last = tr
		return tr, nil
	}

	// "dark" matches the current Workbench theme; it keeps transcript readable without
	// introducing loud colors.
	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("create glamour renderer: %w", err)
	}
	r.byWidth[width] = tr
	r.lastWidth = width
	r.last = tr
	return tr, nil
}
