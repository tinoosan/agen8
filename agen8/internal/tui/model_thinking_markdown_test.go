package tui

import (
	"context"
	"strings"
	"testing"
)

func TestTranscriptThinking_RendersMarkdownSummary(t *testing.T) {
	// Ensure Glamour renders with ANSI styling in tests (so we can assert on faint/dim).
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	m := New(context.Background(), stubRunner{}, nil)
	m.width = 120
	m.height = 24
	m.layout()

	m.transcriptItems = []transcriptItem{
		{kind: transcriptThinking, text: "Thinking…\n\n**Planning next steps**"},
	}
	m.thinkingExpanded = true
	m.rebuildTranscript()

	out := m.transcript.View()
	if strings.Contains(out, "**") {
		t.Fatalf("expected thinking markdown to be rendered (no raw **); got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "planning") {
		t.Fatalf("expected rendered output to contain content; got %q", out)
	}
	// Thinking summaries should be visually distinct (muted palette).
	// Our muted theme uses a gray base color (#8a8a8a -> 138,138,138).
	if !(strings.Contains(out, "38;2;138;138;138") || strings.Contains(out, "38;5;245")) {
		t.Fatalf("expected thinking summary to include muted gray ANSI color; got %q", out)
	}
}
