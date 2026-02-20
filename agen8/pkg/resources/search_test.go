package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDailyMemoryResource_Search_FindsMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.MD"), []byte("master"), 0o644); err != nil {
		t.Fatalf("write MEMORY.MD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-02-04-memory.md"), []byte("My favorite color is blue.\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-02-03-memory.md"), []byte("Nothing here.\n"), 0o644); err != nil {
		t.Fatalf("write daily: %v", err)
	}

	r, err := NewDailyMemoryResource(dir)
	if err != nil {
		t.Fatalf("NewDailyMemoryResource: %v", err)
	}

	results, err := r.Search(context.Background(), "", "blue", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected results")
	}
	if !strings.Contains(strings.ToLower(results[0].Snippet), "blue") {
		t.Fatalf("expected snippet to contain match, got %q", results[0].Snippet)
	}
	if !strings.HasPrefix(results[0].Path, "/memory/") {
		t.Fatalf("expected /memory path, got %q", results[0].Path)
	}
}

func TestDirResource_Search_FindsMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "agent"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "agent", "main.go"), []byte("package agent\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r, err := NewDirResource(dir, "project")
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}

	results, err := r.Search(context.Background(), "", "func Run", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected results")
	}
	if !strings.HasPrefix(results[0].Path, "/project/") {
		t.Fatalf("expected /project path, got %q", results[0].Path)
	}
	if !strings.Contains(results[0].Snippet, "func Run") {
		t.Fatalf("expected snippet to contain match, got %q", results[0].Snippet)
	}
}
