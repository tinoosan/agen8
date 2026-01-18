package tui

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeVFSRunner struct {
	readErr  error
	writes   map[string][]byte
	lastPath string
}

func (r *fakeVFSRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = ctx
	_ = userMsg
	return "", nil
}

func (r *fakeVFSRunner) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	_ = ctx
	_ = maxBytes
	if r.readErr != nil {
		return "", 0, false, r.readErr
	}
	return "", 0, false, fs.ErrNotExist
}

func (r *fakeVFSRunner) WriteVFS(ctx context.Context, path string, data []byte) error {
	_ = ctx
	if r.writes == nil {
		r.writes = make(map[string][]byte)
	}
	r.writes[path] = append([]byte(nil), data...)
	r.lastPath = path
	return nil
}

func TestEditor_OpenNewFile_SetsNotice(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	r := &fakeVFSRunner{readErr: fs.ErrNotExist}
	m := New(context.Background(), r, nil)

	cmd := m.openEditor("/workdir/new.txt")
	msg := cmd()
	model, _ := m.Update(msg)
	m = model.(Model)

	if !m.editorOpen {
		t.Fatalf("expected editorOpen=true")
	}
	if m.editorNotice != "new file" {
		t.Fatalf("expected notice %q, got %q", "new file", m.editorNotice)
	}
}

func TestEditor_Save_WritesViaRunner(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	r := &fakeVFSRunner{readErr: fs.ErrNotExist}
	m := New(context.Background(), r, nil)

	// Open editor (new file).
	cmd := m.openEditor("/workdir/new.txt")
	msg := cmd()
	model, _ := m.Update(msg)
	m = model.(Model)

	m.editorBuf.SetValue("hello\n")
	saveCmd := m.saveEditor()
	if saveCmd == nil {
		t.Fatalf("expected saveCmd")
	}
	saveMsg := saveCmd()

	model, _ = m.Update(saveMsg)
	m = model.(Model)

	if r.lastPath != "/workdir/new.txt" {
		t.Fatalf("lastPath=%q", r.lastPath)
	}
	if got := string(r.writes["/workdir/new.txt"]); got != "hello\n" {
		t.Fatalf("write=%q", got)
	}
	if m.editorNotice != "saved" {
		t.Fatalf("expected notice %q, got %q", "saved", m.editorNotice)
	}
	if m.editorDirty {
		t.Fatalf("expected editorDirty=false")
	}
}

func TestEditor_OpenExistingFile_PropagatesError(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	r := &fakeVFSRunner{readErr: errors.New("boom")}
	m := New(context.Background(), r, nil)

	cmd := m.openEditor("/workdir/existing.txt")
	msg := cmd()
	model, _ := m.Update(msg)
	m = model.(Model)

	if m.editorErr == "" {
		t.Fatalf("expected editorErr")
	}
}

func TestEditor_EscClosesAndRestoresFocus(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	r := &fakeVFSRunner{readErr: fs.ErrNotExist}
	m := New(context.Background(), r, nil)

	cmd := m.openEditor("/workdir/new.txt")
	msg := cmd()
	model, _ := m.Update(msg)
	m = model.(Model)

	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)

	if m.editorOpen {
		t.Fatalf("expected editorOpen=false")
	}
	if m.focus != focusInput {
		t.Fatalf("expected focusInput")
	}
}
