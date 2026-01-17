package tui

import (
	"strings"
	"testing"
)

func TestPreprocessMarkdown_PrettyPrintsJSONFencedBlocks(t *testing.T) {
	in := "before\n```json\n{\"a\":1,\"b\":{\"c\":2}}\n```\nafter\n"
	out := preprocessMarkdown(in)

	// Fence preserved.
	if !strings.Contains(out, "```json") || !strings.Contains(out, "```") {
		t.Fatalf("expected fences to be preserved, got:\n%s", out)
	}

	// JSON indented.
	for _, want := range []string{"{\n", "\"a\": 1", "\"b\": {", "\"c\": 2", "\n}"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPreprocessMarkdown_InvalidJSON_IsLeftRaw(t *testing.T) {
	in := "```json\n{\"a\":\n```\n"
	out := preprocessMarkdown(in)
	if out != in {
		t.Fatalf("expected invalid json block to be unchanged.\nwant:\n%s\ngot:\n%s", in, out)
	}
}

func TestMarkdownRenderer_RendersHeadingsAndLists(t *testing.T) {
	r := newContentRenderer()
	rendered := r.RenderMarkdown("### Title\n\n- item\n", 80)
	if strings.Contains(rendered, "###") {
		t.Fatalf("expected heading markers to be rendered (not shown raw), got:\n%s", rendered)
	}
}
