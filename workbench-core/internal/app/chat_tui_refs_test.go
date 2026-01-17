package app

import (
	"path/filepath"
	"testing"
)

func TestResolveWorkdirVPathFromArg_Unquoted(t *testing.T) {
	vp, display, err := resolveWorkdirVPathFromArg("/tmp", "cmd/main.go")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if vp != "/workdir/cmd/main.go" {
		t.Fatalf("vpath=%q", vp)
	}
	if display != "cmd/main.go" {
		t.Fatalf("display=%q", display)
	}
}

func TestResolveWorkdirVPathFromArg_QuotedAtRef(t *testing.T) {
	vp, display, err := resolveWorkdirVPathFromArg("/tmp", `@"my file.md"`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if vp != "/workdir/my file.md" {
		t.Fatalf("vpath=%q", vp)
	}
	if display != "my file.md" {
		t.Fatalf("display=%q", display)
	}
}

func TestResolveWorkdirVPathFromArg_AbsoluteWithinWorkdir(t *testing.T) {
	workdir := t.TempDir()
	abs := filepath.Join(workdir, "README.md")
	vp, display, err := resolveWorkdirVPathFromArg(workdir, abs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if vp != "/workdir/README.md" {
		t.Fatalf("vpath=%q", vp)
	}
	if display != "README.md" {
		t.Fatalf("display=%q", display)
	}
}

func TestResolveWorkdirVPathFromArg_TraversalRejected(t *testing.T) {
	_, _, err := resolveWorkdirVPathFromArg("/tmp", "../secrets.txt")
	if err == nil {
		t.Fatalf("expected error")
	}
}
