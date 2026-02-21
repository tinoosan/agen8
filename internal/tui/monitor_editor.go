package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type monitorEditorDoneMsg struct {
	path string
	err  error
}

func (m *monitorModel) openComposeEditor(initial string) tea.Cmd {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[editor] error: $VISUAL/$EDITOR not set"}}
		}
	}

	// Persist a compose scratch file under the run workspace so it survives across
	// restarts and stays scoped to the run.
	wsDir := m.workspaceDir()
	_ = os.MkdirAll(wsDir, 0o755)
	composePath := filepath.Join(wsDir, ".monitor-compose.txt")

	// Best-effort seed.
	_ = os.WriteFile(composePath, []byte(initial), 0o644)

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return func() tea.Msg {
			return commandLinesMsg{lines: []string{"[editor] error: invalid $VISUAL/$EDITOR"}}
		}
	}

	cmd := exec.CommandContext(m.ctx, fields[0], append(fields[1:], composePath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return monitorEditorDoneMsg{path: composePath, err: err}
	})
}

func (m *monitorModel) handleEditorDone(msg monitorEditorDoneMsg) {
	if msg.err != nil {
		m.appendAgentOutput("[editor] error: " + msg.err.Error())
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.appendAgentOutput("[editor] read error: " + err.Error())
		return
	}
	// Keep content as-is (multiline); submit handling trims on send.
	m.input.SetValue(string(b))
	// Keep cursor at end for convenience.
	m.input.CursorEnd()
	// Recompute palette since input changed.
	m.updateCommandPalette()
}
