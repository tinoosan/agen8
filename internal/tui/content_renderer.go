package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContentRenderer is the single rendering path for all "content surfaces" in the TUI.
//
// The transcript (user/assistant message bodies), the Activity details panel, and any
// other text surfaces must render through this helper so markdown and code blocks
// look consistent everywhere.
//
// This type is intentionally small:
//   - formatting helpers produce markdown (including fenced code blocks)
//   - rendering turns markdown into styled terminal output via Glamour
//
// Callers should:
//  1. format structured payloads into markdown using the helpers (FormatJSON/FormatCode)
//  2. render with RenderMarkdown
type ContentRenderer struct {
	md            *markdownRenderer
	mdAgent       *markdownRenderer
	mdThinking    *markdownRenderer
	mdCoordinator *markdownRenderer
}

func newContentRenderer() *ContentRenderer {
	return &ContentRenderer{
		md:            newMarkdownRenderer(markdownVariantNormal),
		mdAgent:       newMarkdownRenderer(markdownVariantAgent),
		mdThinking:    newMarkdownRenderer(markdownVariantThinking),
		mdCoordinator: newMarkdownRenderer(markdownVariantCoordinator),
	}
}

// NewContentRenderer constructs the shared markdown/content renderer for TUI packages.
func NewContentRenderer() *ContentRenderer {
	return newContentRenderer()
}

// RenderMarkdown renders markdown into terminal-friendly styled output.
//
// The markdown is preprocessed (e.g. ```json fences are pretty-printed best-effort)
// and then rendered via Glamour with preserved newlines.
func (r *ContentRenderer) RenderMarkdown(markdown string, width int) string {
	if r == nil || r.md == nil {
		return markdown
	}
	return r.md.render(markdown, width)
}

// RenderAgentMarkdown renders markdown for agent responses with white text.
func (r *ContentRenderer) RenderAgentMarkdown(markdown string, width int) string {
	if r == nil || r.mdAgent == nil {
		return markdown
	}
	return r.mdAgent.render(markdown, width)
}

// RenderThinkingMarkdown renders markdown for the "thinking summary" surface.
//
// This is intentionally separate from the main markdown renderer so the UI can
// apply a muted theme for thinking summaries (making them visually distinct from
// normal assistant output).
func (r *ContentRenderer) RenderThinkingMarkdown(markdown string, width int) string {
	if r == nil || r.mdThinking == nil {
		return markdown
	}
	return r.mdThinking.render(markdown, width)
}

// RenderCoordinatorMarkdown renders markdown for the coordinator TUI surfaces.
func (r *ContentRenderer) RenderCoordinatorMarkdown(markdown string, width int) string {
	if r == nil || r.mdCoordinator == nil {
		return markdown
	}
	return r.mdCoordinator.render(markdown, width)
}

// FormatCode wraps code in a fenced block using a "safe fence" length.
//
// Some payloads may contain triple backticks (```), which would break a standard
// markdown fence. We choose a fence length that does not occur in the content.
func FormatCode(lang string, code string) string {
	fence := pickFence(code)
	lang = strings.TrimSpace(lang)
	if lang != "" {
		lang = strings.ToLower(lang)
	}

	var b strings.Builder
	b.WriteString(fence)
	if lang != "" {
		b.WriteString(lang)
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(code, "\n"))
	b.WriteString("\n")
	b.WriteString(fence)
	return b.String()
}

// FormatJSON pretty-prints JSON (best-effort) and wraps it in a fenced ```json block.
//
// If the input is not valid JSON (e.g., truncated), the raw content is rendered.
func FormatJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return FormatCode("json", "")
	}

	if pretty, ok := tryIndentJSON(raw); ok {
		return FormatCode("json", pretty)
	}
	return FormatCode("json", raw)
}

// FormatJSONValue marshals a Go value to pretty JSON and wraps it in a fenced ```json block.
func FormatJSONValue(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return FormatCode("json", fmt.Sprintf("%v", v))
	}
	return FormatCode("json", string(b))
}

func pickFence(content string) string {
	// Start at 3 backticks and increase until we find a delimiter not present in content.
	for n := 3; n <= 8; n++ {
		fence := strings.Repeat("`", n)
		if !strings.Contains(content, fence) {
			return fence
		}
	}
	// Extremely unlikely; fall back to 9.
	return strings.Repeat("`", 9)
}
