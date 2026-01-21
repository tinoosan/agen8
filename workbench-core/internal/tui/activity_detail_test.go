package tui

import (
	"strings"
	"testing"
)

func TestRenderActivityDetailMarkdown_FSWrite_ShowsContentPreview(t *testing.T) {
	a := Activity{
		Kind:        "fs.write",
		Title:       "Write /scratch/example.json",
		Path:        "/scratch/example.json",
		TextPreview: `{"a":1,"b":{"c":2}}`,
		TextIsJSON:  true,
	}

	md := renderActivityDetailMarkdown(a, false, false)
	if !strings.Contains(md, "Written content preview") {
		t.Fatalf("expected content preview section, got:\n%s", md)
	}
	if !strings.Contains(md, "```json") {
		t.Fatalf("expected json code fence, got:\n%s", md)
	}
}
