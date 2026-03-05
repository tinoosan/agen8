package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
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

	results, err := r.Search(context.Background(), "", types.SearchRequest{Query: "blue", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Results) == 0 {
		t.Fatalf("expected results")
	}
	if !strings.Contains(strings.ToLower(results.Results[0].Snippet), "blue") {
		t.Fatalf("expected snippet to contain match, got %q", results.Results[0].Snippet)
	}
	if !strings.HasPrefix(results.Results[0].Path, "/memory/") {
		t.Fatalf("expected /memory path, got %q", results.Results[0].Path)
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

	results, err := r.Search(context.Background(), "", types.SearchRequest{Query: "func Run", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Results) == 0 {
		t.Fatalf("expected results")
	}
	if !strings.HasPrefix(results.Results[0].Path, "/project/") {
		t.Fatalf("expected /project path, got %q", results.Results[0].Path)
	}
	if !strings.Contains(results.Results[0].Snippet, "func Run") {
		t.Fatalf("expected snippet to contain match, got %q", results.Results[0].Snippet)
	}
}

func TestDirResource_Search_FocusedV1FiltersAndCounts(t *testing.T) {
	dir := t.TempDir()
	for _, subdir := range []string{"src", "vendor"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", subdir, err)
		}
	}
	files := map[string]string{
		"src/main.go":      "package main\nfunc Run() {\n\t// TODO(alpha)\n\t// TODO(alpha)\n}\n",
		"src/other.go":     "package main\nfunc Other() {\n\t// TODO(alpha)\n}\n",
		"src/main_test.go": "package main\n// TODO(alpha)\n",
		"vendor/lib.go":    "package vendor\n// TODO(alpha)\n",
		"large.txt":        strings.Repeat("TODO(alpha)\n", 64),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	r, err := NewDirResource(dir, "project")
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}

	results, err := r.Search(context.Background(), "", types.SearchRequest{
		Pattern:         `TODO\([a-z]+\)`,
		Glob:            "**/*.go",
		Exclude:         []string{"vendor/**", "**/*_test.go"},
		PreviewLines:    1,
		IncludeMetadata: true,
		MaxSizeBytes:    128,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results.Total != 2 || results.Returned != 1 || !results.Truncated {
		t.Fatalf("expected truncated total/returned counts, got %+v", results)
	}
	got := results.Results[0]
	if got.Path != "/project/src/main.go" {
		t.Fatalf("expected highest-scoring file first, got %+v", got)
	}
	if len(got.PreviewBefore) != 1 || got.PreviewMatch == "" || len(got.PreviewAfter) != 1 {
		t.Fatalf("expected preview context, got %+v", got)
	}
	if got.SizeBytes == nil || got.Mtime == nil {
		t.Fatalf("expected metadata, got %+v", got)
	}
}

func TestDirResource_Search_QueryIsPlainText(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("TODO(alpha)\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	r, err := NewDirResource(dir, "project")
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}

	results, err := r.Search(context.Background(), "", types.SearchRequest{Query: "TODO(alpha)", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Results) != 1 {
		t.Fatalf("expected literal query match, got %+v", results)
	}
}
