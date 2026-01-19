package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) prefetchWorkdir() tea.Cmd {
	// Workdir can be unknown before first turn. Prefetch it without creating a run.
	return func() tea.Msg {
		wd, err := m.runner.RunTurn(m.ctx, "/pwd")
		return workdirPrefetchMsg{workdir: strings.TrimSpace(wd), err: err}
	}
}

func (m *Model) currentInputValue() string {
	if m.isMulti {
		return m.multiline.Value()
	}
	return m.single.Value()
}

func (m *Model) setCurrentInputValue(v string) {
	if m.isMulti {
		m.multiline.SetValue(v)
	} else {
		m.single.SetValue(v)
	}
}

func (m *Model) clearCurrentInput() {
	if m.isMulti {
		m.multiline.SetValue("")
	} else {
		m.single.SetValue("")
	}
}

func (m *Model) toggleMultiline() {
	m.isMulti = !m.isMulti
	if m.isMulti {
		m.multiline.SetValue(strings.TrimSpace(m.single.Value()))
		m.single.SetValue("")
		m.multiline.Focus()
	} else {
		m.single.SetValue(strings.TrimSpace(m.multiline.Value()))
		m.multiline.SetValue("")
		m.single.Focus()
	}
	m.layout()
}

func (m *Model) submitSingle() tea.Cmd {
	txt := strings.TrimSpace(m.single.Value())
	m.single.SetValue("")
	if txt == "" {
		return nil
	}
	// Intercept `/model` with no args to open picker instead of submitting
	if txt == "/model" {
		return m.openModelPicker()
	}
	return m.submit(txt)
}

func (m *Model) submitMultiline() tea.Cmd {
	txt := strings.TrimSpace(m.multiline.Value())
	m.multiline.SetValue("")
	if txt == "" {
		return nil
	}
	// Intercept `/model` with no args to open picker instead of submitting
	if txt == "/model" {
		return m.openModelPicker()
	}
	return m.submit(txt)
}

func (m *Model) submit(userMsg string) tea.Cmd {
	m.turnInFlight = true
	m.turnStarted = time.Now()
	m.turnTitle = userMsg
	m.turnN++
	m.pendingActionIdx = -1
	m.pendingActionText = ""
	m.waitingForAction = false
	m.streamingItemIdx = -1
	m.streamingBuf = nil
	m.thinkingItemIdx = -1
	m.thinkingStep = 0
	m.thinkingActive = false
	m.thinkingStarted = time.Time{}
	m.thinkingDuration = 0
	m.thinkingSummary = ""

	if m.workflowTitle == "" {
		m.workflowTitle = firstLine(userMsg)
	}

	m.lastTurnUserItemIdx = len(m.transcriptItems)
	m.addTranscriptItem(transcriptItem{kind: transcriptUser, text: userMsg})

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, userMsg)
		_ = final
		return turnDoneMsg{final: final, err: err}
	}
}

func (m *Model) formatThinkingText() string {
	if m == nil {
		return ""
	}
	header := "Thinking…"
	if !m.thinkingActive && m.thinkingDuration > 0 {
		header = "Thought for " + m.thinkingDuration.Truncate(time.Millisecond).String()
	}
	summary := strings.TrimSpace(m.thinkingSummary)
	if summary == "" {
		return header
	}
	if !m.thinkingExpanded {
		return header + "  " + "summary available (Ctrl+Y)"
	}
	// Note: summary is rendered as markdown in the transcript renderer.
	return header + "\n\n" + summary
}

func (m *Model) updateThinkingTranscriptItem() {
	if m == nil {
		return
	}
	if m.thinkingItemIdx < 0 || m.thinkingItemIdx >= len(m.transcriptItems) {
		return
	}
	it := m.transcriptItems[m.thinkingItemIdx]
	if it.kind != transcriptThinking {
		return
	}
	it.text = m.formatThinkingText()
	m.transcriptItems[m.thinkingItemIdx] = it
	wasAtBottom := m.transcript.AtBottom()
	m.rebuildTranscript()
	if wasAtBottom {
		m.transcript.GotoBottom()
	}
}
