package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestResolveAtRefs_ExactWorkdirPath(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "cmd"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "cmd", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "please edit @cmd/main.go", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/workdir/cmd/main.go" {
		t.Fatalf("unexpected vpath: %q", res.Attachments[0].VPath)
	}
	if res.Attachments[0].BytesIncluded == 0 {
		t.Fatalf("expected bytes included > 0")
	}
}

func TestResolveAtRefs_ArtifactFallback(t *testing.T) {
	workdir := t.TempDir()
	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes)

	wsDir := t.TempDir()
	wsRes, err := resources.NewDirResource(wsDir, vfs.MountWorkspace)
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs.Mount(vfs.MountWorkspace, wsRes)

	if err := os.WriteFile(filepath.Join(wsDir, "out.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	artifacts := newArtifactIndex()
	artifacts.ObserveWrite("/workspace/out.txt")

	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "use @out.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/workspace/out.txt" {
		t.Fatalf("unexpected vpath: %q", res.Attachments[0].VPath)
	}
}

func TestResolveAtRefs_FuzzyUnambiguous(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "sub"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "sub", "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "read @hello.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/workdir/sub/hello.txt" {
		t.Fatalf("unexpected vpath: %q", res.Attachments[0].VPath)
	}
}

func TestResolveAtRefs_Ambiguous(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "a"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workdir, "b"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "a", "dup.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "b", "dup.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountWorkdir, workdirRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "read @dup.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 0 {
		t.Fatalf("expected 0 attachments, got %d", len(res.Attachments))
	}
	if len(res.Ambiguous["dup.txt"]) < 2 {
		t.Fatalf("expected ambiguous candidates for dup.txt")
	}
}
