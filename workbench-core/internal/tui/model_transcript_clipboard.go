package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/aymanbagabas/go-osc52/v2"
)

// transcriptForClipboard renders the current transcript timeline into plain text/markdown
// suitable for pasting elsewhere. It intentionally avoids any ANSI styling.
func (m Model) transcriptForClipboard() string {
	if len(m.transcriptItems) == 0 {
		return ""
	}

	var b strings.Builder
	wroteAny := false

	writeBlock := func(title string, body string) {
		body = normalizeNewlinesForClipboard(strings.TrimSpace(body))
		if body == "" {
			return
		}
		if wroteAny {
			b.WriteString("\n\n")
		}
		if strings.TrimSpace(title) != "" {
			b.WriteString("### ")
			b.WriteString(strings.TrimSpace(title))
			b.WriteString("\n\n")
		}
		b.WriteString(body)
		wroteAny = true
	}

	for _, it := range m.transcriptItems {
		switch it.kind {
		case transcriptSpacer:
			// Keep spacing only between actual blocks.
			continue
		case transcriptUser:
			writeBlock("User", it.text)
		case transcriptAgent:
			writeBlock("Assistant", it.text)
		case transcriptThinking:
			writeBlock("Thinking", it.text)
		case transcriptAction:
			body := strings.TrimSpace(it.actionText)
			if strings.TrimSpace(it.actionCompletion) != "" {
				body = strings.TrimSpace(body) + "\n" + strings.TrimSpace(it.actionCompletion)
			}
			writeBlock("Action", body)
		case transcriptFileChange:
			// Already markdown; preserve as-is.
			writeBlock("", it.text)
		case transcriptError:
			writeBlock("Error", it.text)
		}
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return ""
	}
	return out + "\n"
}

func normalizeNewlinesForClipboard(s string) string {
	// Bubble Tea + terminals can emit CRLF depending on environment; normalize for pastes.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

type clipboardDoneMsg struct {
	err error
}

func copyToClipboardCmd(text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// Preserve a trailing newline for nicer paste behavior (and to match transcriptForClipboard()).
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	return func() tea.Msg {
		// Try native clipboard first.
		if err := clipboard.WriteAll(text); err == nil {
			return clipboardDoneMsg{}
		}

		// Fallback: OSC52 (works in many SSH/tmux scenarios if the terminal allows it).
		seq := osc52.New(text)
		if os.Getenv("TMUX") != "" {
			seq = seq.Tmux()
		} else if os.Getenv("STY") != "" {
			seq = seq.Screen()
		}
		_, err := fmt.Fprint(os.Stderr, seq)
		return clipboardDoneMsg{err: err}
	}
}

