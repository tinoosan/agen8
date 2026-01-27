package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

type markdownVariant int

const (
	markdownVariantNormal markdownVariant = iota
	markdownVariantAgent
	markdownVariantThinking
)

// markdownRenderer caches Glamour renderers keyed by wrap width.
//
// Glamour renderers are relatively expensive to construct. The TUI re-renders the transcript
// on resize and when toggling the right pane, so caching keeps the UI responsive.
type markdownRenderer struct {
	variant markdownVariant

	mu        sync.Mutex
	byWidth   map[int]*glamour.TermRenderer
	lastWidth int
	last      *glamour.TermRenderer
}

func newMarkdownRenderer(variant markdownVariant) *markdownRenderer {
	return &markdownRenderer{
		variant: variant,
		byWidth: map[int]*glamour.TermRenderer{},
	}
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

	style := workbenchMarkdownStyle(r.variant)

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

func uintPtr(v uint) *uint       { return &v }
func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

func workbenchMarkdownStyle(variant markdownVariant) ansi.StyleConfig {
	// "dark" matches the current Workbench theme; it keeps transcript readable without
	// introducing loud colors.
	//
	// We also tweak the code block styling slightly to make it read as "code" (not prose)
	// and we preserve newlines so pasted tasks/snippets keep their structure.
	style := styles.DarkStyleConfig

	// Render markdown as markdown (hide raw markers like "###" and "**").
	style.H1.StylePrimitive.Prefix = ""
	style.H2.StylePrimitive.Prefix = ""
	style.H3.StylePrimitive.Prefix = ""
	style.H4.StylePrimitive.Prefix = ""
	style.H5.StylePrimitive.Prefix = ""
	style.H6.StylePrimitive.Prefix = ""

	style.Strong.BlockPrefix = ""
	style.Strong.BlockSuffix = ""
	style.Strong.Bold = boolPtr(true)

	style.Emph.BlockPrefix = ""
	style.Emph.BlockSuffix = ""
	style.Emph.Italic = boolPtr(true)

	style.Strikethrough.BlockPrefix = ""
	style.Strikethrough.BlockSuffix = ""

	style.Code.StylePrimitive.BlockPrefix = ""
	style.Code.StylePrimitive.BlockSuffix = ""

	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.Indent = uintPtr(0)
	// Keep code blocks tight to preceding content (diff headers, etc.).
	style.CodeBlock.StylePrimitive.BlockPrefix = ""
	style.CodeBlock.StylePrimitive.BlockSuffix = "\n"
	// Subtle background for code blocks to improve scannability.
	style.CodeBlock.StylePrimitive.BackgroundColor = stringPtr("#1c1f2b")
	style.CodeBlock.StylePrimitive.Color = stringPtr("#eaeaea")

	if variant == markdownVariantAgent {
		// Set text colors to white for agent responses
		style.Document.Color = stringPtr("#ffffff")
		style.Paragraph.Color = stringPtr("#ffffff")
		style.Text.Color = stringPtr("#ffffff")
		style.Heading.Color = stringPtr("#ffffff")
		style.H1.Color = stringPtr("#ffffff")
		style.H2.Color = stringPtr("#ffffff")
		style.H3.Color = stringPtr("#ffffff")
		style.H4.Color = stringPtr("#ffffff")
		style.H5.Color = stringPtr("#ffffff")
		style.H6.Color = stringPtr("#ffffff")
		style.List.Color = stringPtr("#ffffff")
		style.Item.Color = stringPtr("#ffffff")
		style.Enumeration.Color = stringPtr("#ffffff")
		style.Link.Color = stringPtr("#ffffff")
		style.LinkText.Color = stringPtr("#ffffff")
		style.Emph.Color = stringPtr("#ffffff")
		style.Strong.Color = stringPtr("#ffffff")
		style.Strikethrough.Color = stringPtr("#ffffff")
	}

	if variant == markdownVariantThinking {
		applyThinkingMutedTheme(&style)
	}
	return style
}

func applyThinkingMutedTheme(style *ansi.StyleConfig) {
	if style == nil {
		return
	}
	// Goal: make the "thinking summary" visually distinct from normal assistant output.
	// Use Faint (SGR 2) + a muted gray palette so markdown-emitted styling doesn't
	// overpower the transcript's dim wrapper.
	const base = "#8a8a8a"
	const code = "#a0a0a0"
	const codeBg = "#141414"

	// Document/text surfaces.
	style.Document.Color = stringPtr(base)
	style.Document.Faint = boolPtr(true)
	style.Paragraph.Color = stringPtr(base)
	style.Paragraph.Faint = boolPtr(true)
	style.Text.Color = stringPtr(base)
	style.Text.Faint = boolPtr(true)

	// Headings: keep readable but muted.
	style.Heading.Color = stringPtr(base)
	style.Heading.Faint = boolPtr(true)
	style.H1.Color = stringPtr(base)
	style.H1.Faint = boolPtr(true)
	style.H2.Color = stringPtr(base)
	style.H2.Faint = boolPtr(true)
	style.H3.Color = stringPtr(base)
	style.H3.Faint = boolPtr(true)
	style.H4.Color = stringPtr(base)
	style.H4.Faint = boolPtr(true)
	style.H5.Color = stringPtr(base)
	style.H5.Faint = boolPtr(true)
	style.H6.Color = stringPtr(base)
	style.H6.Faint = boolPtr(true)

	// Inline styling.
	style.Emph.Color = stringPtr(base)
	style.Emph.Faint = boolPtr(true)
	style.Strong.Color = stringPtr(base)
	style.Strong.Faint = boolPtr(true)
	style.Strikethrough.Color = stringPtr(base)
	style.Strikethrough.Faint = boolPtr(true)

	// Lists and link chrome.
	style.List.Color = stringPtr(base)
	style.List.Faint = boolPtr(true)
	style.Item.Color = stringPtr(base)
	style.Item.Faint = boolPtr(true)
	style.Enumeration.Color = stringPtr(base)
	style.Enumeration.Faint = boolPtr(true)
	style.Link.Color = stringPtr(base)
	style.Link.Faint = boolPtr(true)
	style.LinkText.Color = stringPtr(base)
	style.LinkText.Faint = boolPtr(true)

	// Code: slightly higher contrast, but still muted/faint.
	style.Code.Color = stringPtr(code)
	style.Code.Faint = boolPtr(true)
	style.CodeBlock.StylePrimitive.Color = stringPtr(code)
	style.CodeBlock.StylePrimitive.BackgroundColor = stringPtr(codeBg)
	style.CodeBlock.StylePrimitive.Faint = boolPtr(true)
}

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
