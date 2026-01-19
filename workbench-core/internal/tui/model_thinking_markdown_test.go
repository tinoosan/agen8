package tui

import (
	"context"
	"strings"
	"testing"
)

func TestTranscriptThinking_RendersMarkdownSummary(t *testing.T) {
	t.Parallel()

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
}
