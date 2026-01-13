package resources

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestList_IncludesBuiltinsAndDisk(t *testing.T) {
	tmp := t.TempDir()

	tr := &ToolsResource{
		BaseDir: tmp,
		Mount:   "tools",
		BuiltinRegistry: map[string]BuiltinTool{
			"github.com.acme.stock": {Manifest: []byte(`{"id":"github.com.acme.stock"}`)},
			"github.com.dupe.tool":  {Manifest: []byte(`{"id":"github.com.dupe.tool","source":"builtin"}`)},
		},
	}

	// Disk tool with manifest
	if err := os.MkdirAll(filepath.Join(tmp, "github.com.other.stock"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "github.com.other.stock", "manifest.json"), []byte(`{"id":"github.com.other.stock"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Disk dir without manifest should be ignored
	if err := os.MkdirAll(filepath.Join(tmp, "github.com.no.manifest"), 0755); err != nil {
		t.Fatal(err)
	}

	// Disk file should be ignored
	if err := os.WriteFile(filepath.Join(tmp, "not-a-dir"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Collision: disk has same tool ID as builtin; builtin wins and list only once
	if err := os.MkdirAll(filepath.Join(tmp, "github.com.dupe.tool"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "github.com.dupe.tool", "manifest.json"), []byte(`{"id":"github.com.dupe.tool","source":"disk"}`), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := tr.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir {
			t.Fatalf("expected IsDir=true for %q", e.Path)
		}
		got = append(got, e.Path)
	}

	want := []string{
		"github.com.acme.stock",
		"github.com.dupe.tool",
		"github.com.other.stock",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List mismatch:\nwant=%v\ngot=%v", want, got)
	}
}

func TestRead_BuiltinOverridesDisk(t *testing.T) {
	tmp := t.TempDir()

	tr := &ToolsResource{
		BaseDir: tmp,
		Mount:   "tools",
		BuiltinRegistry: map[string]BuiltinTool{
			"github.com.dupe.tool": {Manifest: []byte(`{"id":"github.com.dupe.tool","source":"builtin"}`)},
		},
	}

	if err := os.MkdirAll(filepath.Join(tmp, "github.com.dupe.tool"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "github.com.dupe.tool", "manifest.json"), []byte(`{"id":"github.com.dupe.tool","source":"disk"}`), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := tr.Read("github.com.dupe.tool")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(b) != `{"id":"github.com.dupe.tool","source":"builtin"}` {
		t.Fatalf("unexpected bytes: %q", string(b))
	}
}

func TestRead_RejectsNestedPaths(t *testing.T) {
	tr := &ToolsResource{
		BaseDir:         t.TempDir(),
		Mount:           "tools",
		BuiltinRegistry: map[string]BuiltinTool{},
	}

	if _, err := tr.Read("github.com.acme.stock/bin/x"); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := tr.Read("github.com.acme.stock/../secrets"); err == nil {
		t.Fatalf("expected error")
	}
}
