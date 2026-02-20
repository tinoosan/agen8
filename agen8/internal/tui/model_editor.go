package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/vfsutil"
)

func (m Model) renderEditorView() string {
	header := m.renderHeader()

	title := m.editorTitle()
	w := max(1, m.width-2)
	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0")).
		Padding(0, 1).
		Render(kit.TruncateMiddle(title, w))

	bodyH := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(bar)-2)
	m.editorBuf.SetHeight(bodyH)
	m.editorBuf.SetWidth(max(1, m.width-2))
	editor := lipgloss.NewStyle().Padding(0, 1).Render(m.editorBuf.View())

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Padding(0, 1).
		Render("ctrl+o save  esc cancel")

	return header + "\n" + bar + "\n" + editor + "\n" + footer
}

func (m Model) editorTitle() string {
	vp := strings.TrimSpace(m.editorVPath)
	name := vp
	switch {
	case strings.HasPrefix(vp, "/project/"):
		name = strings.TrimPrefix(vp, "/project/")
	case strings.HasPrefix(vp, "/workspace/"):
		name = strings.TrimPrefix(vp, "/workspace/")
	}
	title := "Editing: " + name
	if m.editorDirty {
		title += " *"
	}
	if strings.TrimSpace(m.editorNotice) != "" {
		title += " · " + strings.TrimSpace(m.editorNotice)
	}
	if strings.TrimSpace(m.editorErr) != "" {
		title += " · error: " + strings.TrimSpace(m.editorErr)
	}
	return title
}

func (m *Model) openEditor(vpath string) tea.Cmd {
	// Prefer the user's external editor when configured.
	//
	// /editor is a host UX convenience command; it should open $VISUAL/$EDITOR
	// rather than forcing users into a bespoke in-TUI editor.
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		if cmd, err := m.editorExecCmd(editor, vpath); err == nil && cmd != nil {
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return editorExternalDoneMsg{vpath: vpath, err: err}
			})
		}
	}
	m.editorComposeOnClose = false
	return m.openInternalEditor(vpath)
}

func (m *Model) openEditorAbs(absPath string) tea.Cmd {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return nil
	}
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		if cmd, err := m.editorExecCmdAbs(editor, absPath); err == nil && cmd != nil {
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return editorExternalDoneMsg{vpath: absPath, err: err}
			})
		}
	}
	m.editorComposeOnClose = false
	return m.openInternalEditorAbs(absPath)
}

func (m *Model) openComposeEditor(absPath string) tea.Cmd {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return nil
	}
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		if cmd, err := m.editorExecCmdAbs(editor, absPath); err == nil && cmd != nil {
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return editorExternalDoneMsg{vpath: absPath, err: err}
			})
		}
	}
	m.editorComposeOnClose = true
	return m.openInternalEditorAbs(absPath)
}

func (m *Model) openInternalEditor(vpath string) tea.Cmd {
	m.editorOpen = true
	m.editorVPath = strings.TrimSpace(vpath)
	m.editorDirty = false
	m.editorErr = ""
	m.editorNotice = ""
	m.editorBuf.SetValue("")
	m.editorBuf.Focus()
	m.layout()

	return func() tea.Msg {
		acc, ok := m.runner.(vfsAccessor)
		if !ok {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
		}
		txt, _, truncated, err := acc.ReadVFS(m.ctx, vpath, 512*1024)
		if err != nil {
			// Missing file is a valid workflow: open a new file and allow saving it.
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
				return editorLoadMsg{vpath: vpath, content: "", notice: "new file"}
			}
			return editorLoadMsg{vpath: vpath, err: err}
		}
		if truncated {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("file too large to edit in TUI (truncated)")}
		}
		return editorLoadMsg{vpath: vpath, content: txt}
	}
}

func (m *Model) openInternalEditorAbs(absPath string) tea.Cmd {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return nil
	}
	m.editorOpen = true
	m.editorVPath = absPath
	m.editorDirty = false
	m.editorErr = ""
	m.editorNotice = ""
	m.editorBuf.SetValue("")
	m.editorBuf.Focus()
	m.layout()

	return func() tea.Msg {
		b, err := os.ReadFile(absPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
				return editorLoadMsg{vpath: absPath, content: "", notice: "new file"}
			}
			return editorLoadMsg{vpath: absPath, err: err}
		}
		if len(b) > 512*1024 {
			return editorLoadMsg{vpath: absPath, err: fmt.Errorf("file too large to edit in TUI")}
		}
		return editorLoadMsg{vpath: absPath, content: string(b)}
	}
}

func (m *Model) editorExecCmd(editor string, vpath string) (*exec.Cmd, error) {
	editor = strings.TrimSpace(editor)
	vpath = strings.TrimSpace(vpath)
	if editor == "" || vpath == "" {
		return nil, fmt.Errorf("editor and vpath are required")
	}
	if !strings.HasPrefix(vpath, "/project/") {
		return nil, fmt.Errorf("external editor only supports /project paths")
	}
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		return nil, fmt.Errorf("workdir is unknown")
	}

	sub := strings.TrimPrefix(vpath, "/project/")
	clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
	if err != nil || clean == "" || clean == "." {
		return nil, fmt.Errorf("invalid workdir path: %s", vpath)
	}
	abs := filepath.Join(workdir, filepath.FromSlash(clean))

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid editor")
	}
	cmd := exec.CommandContext(m.ctx, fields[0], append(fields[1:], abs)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (m *Model) editorExecCmdAbs(editor string, absPath string) (*exec.Cmd, error) {
	editor = strings.TrimSpace(editor)
	absPath = strings.TrimSpace(absPath)
	if editor == "" || absPath == "" {
		return nil, fmt.Errorf("editor and path are required")
	}
	if !filepath.IsAbs(absPath) {
		return nil, fmt.Errorf("path must be absolute: %s", absPath)
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid editor")
	}
	cmd := exec.CommandContext(m.ctx, fields[0], append(fields[1:], absPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (m *Model) loadComposeBuffer(vpath string) tea.Cmd {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		vpath = composeVPath
	}
	// Compose buffer always lives under /project.
	if !strings.HasPrefix(vpath, "/project/") {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("compose buffer must be under /project")}
		}
	}
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("workdir is unknown")}
		}
	}
	sub := strings.TrimPrefix(vpath, "/project/")
	clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
	if err != nil || clean == "" || clean == "." {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("invalid compose path: %s", vpath)}
		}
	}
	abs := filepath.Join(workdir, filepath.FromSlash(clean))
	return func() tea.Msg {
		b, err := os.ReadFile(abs)
		if err != nil {
			return editorComposeLoadMsg{vpath: composeVPath, err: err}
		}
		return editorComposeLoadMsg{vpath: composeVPath, text: string(b)}
	}
}

func (m *Model) loadComposeBufferAbs(absPath string) tea.Cmd {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: absPath, err: fmt.Errorf("compose path is empty")}
		}
	}
	return func() tea.Msg {
		b, err := os.ReadFile(absPath)
		if err != nil {
			return editorComposeLoadMsg{vpath: absPath, err: err}
		}
		return editorComposeLoadMsg{vpath: absPath, text: string(b)}
	}
}

func (m *Model) openComposeEditorPrefill() tea.Cmd {
	// Read current input value (single or multiline).
	cur := ""
	if m.isMulti {
		cur = m.multiline.Value()
	} else {
		cur = m.single.Value()
	}

	if strings.TrimSpace(m.dataDir) != "" {
		composeAbs := filepath.Join(strings.TrimSpace(m.dataDir), "compose.md")
		_ = os.MkdirAll(filepath.Dir(composeAbs), 0755)
		_ = os.WriteFile(composeAbs, []byte(cur), 0644)
		m.externalEditorComposeVPath = ""
		m.externalEditorComposePath = composeAbs
		return m.openComposeEditor(composeAbs)
	}

	// Fallback: older behavior used the workdir-local compose file.
	// Require a known workdir so we can write the compose buffer.
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		// Best-effort: try to prefetch it; otherwise no-op.
		return m.prefetchWorkdir()
	}

	composeRel := filepath.FromSlash(".agen8/compose.md")
	composeAbs := filepath.Join(workdir, composeRel)
	_ = os.MkdirAll(filepath.Dir(composeAbs), 0755)
	_ = os.WriteFile(composeAbs, []byte(cur), 0644)

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		// No external editor configured; fall back to internal editor UX.
		m.externalEditorComposeVPath = composeVPath
		m.externalEditorComposePath = ""
		m.editorComposeOnClose = true
		return m.openInternalEditor(composeVPath)
	}

	m.externalEditorComposeVPath = composeVPath
	m.externalEditorComposePath = ""
	if cmd, err := m.editorExecCmd(editor, composeVPath); err == nil && cmd != nil {
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return editorExternalDoneMsg{vpath: composeVPath, err: err}
		})
	}
	return nil
}

func (m *Model) saveEditor() tea.Cmd {
	vpath := strings.TrimSpace(m.editorVPath)
	if vpath == "" {
		return nil
	}
	data := []byte(m.editorBuf.Value())
	return func() tea.Msg {
		if isVFSMountPath(vpath) {
			acc, ok := m.runner.(vfsAccessor)
			if !ok {
				return editorSaveMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
			}
			if err := acc.WriteVFS(m.ctx, vpath, data); err != nil {
				return editorSaveMsg{vpath: vpath, err: err}
			}
			return editorSaveMsg{vpath: vpath}
		}
		// Absolute OS path (compose buffer).
		_ = os.MkdirAll(filepath.Dir(vpath), 0755)
		if err := os.WriteFile(vpath, data, 0644); err != nil {
			return editorSaveMsg{vpath: vpath, err: err}
		}
		return editorSaveMsg{vpath: vpath}
	}
}

func isVFSMountPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	// Treat known VFS mounts as virtual paths.
	return hasMountPrefix(p, "project") ||
		hasMountPrefix(p, "workspace") ||
		hasMountPrefix(p, "results") ||
		hasMountPrefix(p, "log") ||
		hasMountPrefix(p, "tools") ||
		hasMountPrefix(p, "memory") ||
		hasMountPrefix(p, "profile") ||
		hasMountPrefix(p, "history")
}

func hasMountPrefix(p string, mount string) bool {
	mount = strings.TrimSpace(mount)
	if mount == "" {
		return false
	}
	base := "/" + mount
	return p == base || strings.HasPrefix(p, base+"/")
}
