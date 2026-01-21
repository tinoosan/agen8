package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestTUITurnRunner_CD_RebindsWorkdirAndUpdatesBuiltins(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	fs := vfs.NewFS()
	workdirRes1, err := resources.NewWorkdirResource(dir1)
	if err != nil {
		t.Fatalf("NewWorkdirResource(dir1): %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes1)

	// Seed builtin invokers with dir1.
	builtins := tools.MapRegistry{
		types.ToolID("builtin.shell"):   tools.NewBuiltinShellInvoker(dir1, nil),
		types.ToolID("builtin.ripgrep"): tools.NewBuiltinRipgrepInvoker(dir1),
	}

	var got []events.Event
	r := &tuiTurnRunner{
		fs:               fs,
		workdirBase:      dir1,
		builtinInvokers:  builtins,
		mustEmit:         func(_ context.Context, ev events.Event) { got = append(got, ev) },
		constructor:      nil,
		artifacts:        nil,
		baseSystemPrompt: "",
	}

	if _, handled := r.handleSlashCommand("/cd " + dir2); !handled {
		t.Fatalf("expected /cd to be handled")
	}

	// VFS writes should go to the new directory after /cd.
	if err := fs.Write("/workdir/hello.txt", []byte("hi")); err != nil {
		t.Fatalf("fs.Write(/workdir/hello.txt): %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir2, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile(dir2/hello.txt): %v", err)
	}
	if string(b) != "hi" {
		t.Fatalf("dir2/hello.txt=%q, want %q", string(b), "hi")
	}

	// Builtin sandbox roots should follow the active workdir.
	sh := builtins[types.ToolID("builtin.shell")].(*tools.BuiltinShellInvoker)
	if sh.RootDir != dir2 {
		t.Fatalf("builtin.shell root=%q, want %q", sh.RootDir, dir2)
	}
	rg := builtins[types.ToolID("builtin.ripgrep")].(*tools.BuiltinRipgrepInvoker)
	if rg.RootDir != dir2 {
		t.Fatalf("builtin.ripgrep root=%q, want %q", rg.RootDir, dir2)
	}

	// Ensure we emitted a workdir.changed event.
	found := false
	for _, ev := range got {
		if ev.Type == "workdir.changed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a workdir.changed event")
	}
}

func TestTUITurnRunner_CD_InvalidDirDoesNotChange(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	fs := vfs.NewFS()
	workdirRes1, err := resources.NewWorkdirResource(dir1)
	if err != nil {
		t.Fatalf("NewWorkdirResource(dir1): %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes1)

	builtins := tools.MapRegistry{
		types.ToolID("builtin.shell"): tools.NewBuiltinShellInvoker(dir1, nil),
	}

	r := &tuiTurnRunner{
		fs:              fs,
		workdirBase:     dir1,
		builtinInvokers: builtins,
		mustEmit:        func(_ context.Context, _ events.Event) {},
	}

	nonexistent := filepath.Join(dir1, "does-not-exist")
	if _, handled := r.handleSlashCommand("/cd " + nonexistent); !handled {
		t.Fatalf("expected /cd to be handled")
	}
	if r.workdirBase != dir1 {
		t.Fatalf("workdirBase=%q, want %q", r.workdirBase, dir1)
	}
}

func TestLazyRunner_CD_DoesNotInitializeSession(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	ch := make(chan events.Event, 10)

	r := &lazyNewSessionTurnRunner{
		ctx:  context.Background(),
		opts: resolveRunChatOptions(WithWorkDir(dir1)),
		evCh: ch,
	}

	final, err := r.RunTurn(context.Background(), "/cd "+dir2)
	if err != nil {
		t.Fatalf("RunTurn(/cd): %v", err)
	}
	if final != "" {
		t.Fatalf("final=%q, want empty", final)
	}
	if r.initialized {
		t.Fatalf("expected runner to remain uninitialized")
	}
	if r.opts.WorkDir != dir2 {
		t.Fatalf("opts.WorkDir=%q, want %q", r.opts.WorkDir, dir2)
	}

	select {
	case ev := <-ch:
		if ev.Type != "workdir.changed" {
			t.Fatalf("event type=%q, want %q", ev.Type, "workdir.changed")
		}
	default:
		t.Fatalf("expected a workdir.changed event")
	}
}
