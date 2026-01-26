package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/atref"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/vfs"
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
	fs.Mount(vfs.MountProject, workdirRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "please edit @cmd/main.go", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/project/cmd/main.go" {
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
	fs.Mount(vfs.MountProject, workdirRes)

	wsDir := t.TempDir()
	wsRes, err := resources.NewDirResource(wsDir, vfs.MountScratch)
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs.Mount(vfs.MountScratch, wsRes)

	if err := os.MkdirAll(filepath.Join(wsDir, "sub"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "sub", "out.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	artifacts := newArtifactIndex()
	artifacts.ObserveWrite("/scratch/sub/out.txt")

	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "use @out.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/scratch/sub/out.txt" {
		t.Fatalf("unexpected vpath: %q", res.Attachments[0].VPath)
	}
}

func TestResolveAtRefs_ExactWorkspacePath(t *testing.T) {
	workdir := t.TempDir()
	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountProject, workdirRes)

	wsDir := t.TempDir()
	wsRes, err := resources.NewDirResource(wsDir, vfs.MountScratch)
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs.Mount(vfs.MountScratch, wsRes)

	if err := os.WriteFile(filepath.Join(wsDir, "out.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "use @out.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/scratch/out.txt" {
		t.Fatalf("unexpected vpath: %q", res.Attachments[0].VPath)
	}
}

func TestResolveAtRefs_ExplicitVPaths(t *testing.T) {
	workdir := t.TempDir()
	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountProject, workdirRes)

	wsDir := t.TempDir()
	wsRes, err := resources.NewDirResource(wsDir, vfs.MountScratch)
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs.Mount(vfs.MountScratch, wsRes)

	if err := os.WriteFile(filepath.Join(workdir, "out.txt"), []byte("workdir\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "out.txt"), []byte("workspace\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "use @/scratch/out.txt and @/project/out.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(res.Attachments))
	}
	got := map[string]bool{}
	for _, a := range res.Attachments {
		got[a.VPath] = true
	}
	if !got["/scratch/out.txt"] || !got["/project/out.txt"] {
		t.Fatalf("unexpected vpaths: %+v", got)
	}
}

func TestResolveAtRefs_FuzzyCrossRootAmbiguous(t *testing.T) {
	workdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, "a"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "a", "dup.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wsDir, "b"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "b", "dup.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	workdirRes, err := resources.NewWorkdirResource(workdir)
	if err != nil {
		t.Fatalf("NewWorkdirResource: %v", err)
	}
	fs.Mount(vfs.MountProject, workdirRes)

	wsRes, err := resources.NewDirResource(wsDir, vfs.MountScratch)
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs.Mount(vfs.MountScratch, wsRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "read @dup.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 0 {
		t.Fatalf("expected 0 attachments, got %d", len(res.Attachments))
	}
	cands := res.Ambiguous["dup.txt"]
	if len(cands) < 2 {
		t.Fatalf("expected ambiguous candidates for dup.txt")
	}
	joined := strings.Join(cands, ",")
	if !strings.Contains(joined, "/project/a/dup.txt") || !strings.Contains(joined, "/scratch/b/dup.txt") {
		t.Fatalf("unexpected candidates: %v", cands)
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
	fs.Mount(vfs.MountProject, workdirRes)

	artifacts := newArtifactIndex()
	res, err := ResolveAtRefs(fs, workdirRes.BaseDir, artifacts, "read @hello.txt", 6, 48*1024, 12*1024)
	if err != nil {
		t.Fatalf("ResolveAtRefs: %v", err)
	}
	if len(res.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(res.Attachments))
	}
	if res.Attachments[0].VPath != "/project/sub/hello.txt" {
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
	fs.Mount(vfs.MountProject, workdirRes)

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

func TestExtractAtRefs_Quoted(t *testing.T) {
	t.Parallel()

	got := atref.ExtractAtRefs(`please open @"my file.md" and @‘notes one.md’ and @cmd/main.go`)
	if len(got) != 3 {
		t.Fatalf("got %d tokens, want %d (%v)", len(got), 3, got)
	}
	if got[0] != "my file.md" {
		t.Fatalf("got[0]=%q, want %q", got[0], "my file.md")
	}
	if got[1] != "notes one.md" {
		t.Fatalf("got[1]=%q, want %q", got[1], "notes one.md")
	}
	if got[2] != "cmd/main.go" {
		t.Fatalf("got[2]=%q, want %q", got[2], "cmd/main.go")
	}
}
