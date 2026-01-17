package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
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
	md = preprocessMarkdown(md)
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
	//
	// We also tweak the code block styling slightly to make it read as "code" (not prose)
	// and we preserve newlines so pasted tasks/snippets keep their structure.
	style := styles.DarkStyleConfig
	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.Indent = uintPtr(0)
	style.CodeBlock.StylePrimitive.BlockPrefix = "\n"
	style.CodeBlock.StylePrimitive.BlockSuffix = "\n"

	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return nil, fmt.Errorf("create glamour renderer: %w", err)
	}
	r.byWidth[width] = tr
	r.lastWidth = width
	r.last = tr
	return tr, nil
}

func uintPtr(v uint) *uint { return &v }

// preprocessMarkdown applies small, deterministic markdown rewrites that improve readability
// in the terminal without changing the meaning of the content.
//
// Currently:
// - ```json fenced blocks are pretty-printed via json.Indent (best-effort).
func preprocessMarkdown(md string) string {
	lines := strings.Split(md, "\n")

	var out []string
	out = make([]string, 0, len(lines))

	inFence := false
	fence := ""
	lang := ""
	var buf []string

	for _, line := range lines {
		if !inFence {
			f, l, ok := parseFenceStart(line)
			if !ok {
				out = append(out, line)
				continue
			}
			inFence = true
			fence = f
			lang = l
			buf = buf[:0]
			out = append(out, line) // keep opener as-is
			continue
		}

		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, fence) && strings.TrimSpace(trim[len(fence):]) == "" {
			// Close fence.
			code := strings.Join(buf, "\n")
			if strings.EqualFold(lang, "json") {
				if pretty, ok := tryIndentJSON(code); ok {
					code = pretty
				}
			}
			if code != "" {
				out = append(out, strings.Split(code, "\n")...)
			}
			out = append(out, line) // keep closer as-is

			inFence = false
			fence = ""
			lang = ""
			buf = buf[:0]
			continue
		}

		buf = append(buf, line)
	}

	// Unclosed fence: keep content as raw lines.
	if inFence && len(buf) > 0 {
		out = append(out, buf...)
	}

	return strings.Join(out, "\n")
}

func parseFenceStart(line string) (fence string, lang string, ok bool) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "```") {
		return "", "", false
	}
	n := 0
	for n < len(trim) && trim[n] == '`' {
		n++
	}
	if n < 3 {
		return "", "", false
	}
	fence = trim[:n]
	rest := strings.TrimSpace(trim[n:])
	if rest == "" {
		return fence, "", true
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return fence, "", true
	}
	return fence, strings.ToLower(fields[0]), true
}

func tryIndentJSON(code string) (string, bool) {
	raw := bytes.TrimSpace([]byte(code))
	if len(raw) == 0 {
		return code, true
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}
