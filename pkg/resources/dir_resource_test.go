package resources

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/agen8/pkg/vfsutil"
)

func TestDirResource_TraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{BaseDir: tmpDir, Mount: "workspace"}

	if err := dr.Write("../x", []byte("nope")); err == nil || !errors.Is(err, vfsutil.ErrEscapesRoot) {
		t.Fatalf("Write traversal should be rejected with escape error, got: %v", err)
	}
	if _, err := dr.Read("/etc/passwd"); err == nil || !errors.Is(err, vfsutil.ErrInvalidPath) {
		t.Fatalf("Read absolute path should be rejected with absolute-path error, got: %v", err)
	}
}

func TestWriteRead_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	content := []byte("hi")
	filename := "notes.md"

	if err := dr.Write(filename, content); err != nil {
		t.Fatalf("Write(%q) failed: %v", filename, err)
	}

	readContent, err := dr.Read(filename)
	if err != nil {
		t.Fatalf("Read(%q) failed: %v", filename, err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Read(%q) = %q, want %q", filename, readContent, content)
	}
}

func TestAppend_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	filename := "log.txt"
	firstLine := []byte("a\n")
	secondLine := []byte("b\n")

	if err := dr.Append(filename, firstLine); err != nil {
		t.Fatalf("First Append(%q) failed: %v", filename, err)
	}

	if err := dr.Append(filename, secondLine); err != nil {
		t.Fatalf("Second Append(%q) failed: %v", filename, err)
	}

	content, err := dr.Read(filename)
	if err != nil {
		t.Fatalf("Read(%q) failed: %v", filename, err)
	}

	expected := "a\nb\n"
	if string(content) != expected {
		t.Errorf("Read(%q) = %q, want %q", filename, content, expected)
	}
}

func TestList_ReturnsCorrectPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	if err := dr.Write("notes.md", []byte("test content")); err != nil {
		t.Fatalf("Failed to create notes.md: %v", err)
	}

	if err := dr.Write("reports/q1.md", []byte("Q1 report")); err != nil {
		t.Fatalf("Failed to create reports/q1.md: %v", err)
	}

	t.Run("list root", func(t *testing.T) {
		entries, err := dr.List("")
		if err != nil {
			t.Fatalf("List(\"\") failed: %v", err)
		}

		if len(entries) != 2 {
			t.Errorf("List(\"\") returned %d entries, want 2", len(entries))
		}

		foundNotes := false
		foundReports := false
		for _, e := range entries {
			if e.Path == "notes.md" && !e.IsDir {
				foundNotes = true
			}
			if e.Path == "reports" && e.IsDir {
				foundReports = true
			}
		}

		if !foundNotes {
			t.Error("List(\"\") should contain notes.md")
		}
		if !foundReports {
			t.Error("List(\"\") should contain reports directory")
		}
	})

	t.Run("list reports subdirectory", func(t *testing.T) {
		entries, err := dr.List("reports")
		if err != nil {
			t.Fatalf("List(\"reports\") failed: %v", err)
		}

		if len(entries) != 1 {
			t.Errorf("List(\"reports\") returned %d entries, want 1", len(entries))
		}

		if entries[0].Path != "reports/q1.md" {
			t.Errorf("List(\"reports\") entry path = %q, want %q", entries[0].Path, "reports/q1.md")
		}

		if entries[0].IsDir {
			t.Error("List(\"reports\") q1.md should not be marked as directory")
		}
	})

	t.Run("entries have metadata", func(t *testing.T) {
		entries, err := dr.List("")
		if err != nil {
			t.Fatalf("List(\"\") failed: %v", err)
		}

		for _, e := range entries {
			if !e.IsDir {
				if !e.HasSize {
					t.Errorf("File entry %q should have HasSize=true", e.Path)
				}
				if !e.HasModTime {
					t.Errorf("File entry %q should have HasModTime=true", e.Path)
				}
				if e.Size < 0 {
					t.Errorf("File entry %q should have Size >= 0, got %d", e.Path, e.Size)
				}
			}
		}
	})
}

func TestWrite_CreatesParentDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	nestedPath := "a/b/c/file.txt"
	content := []byte("nested content")

	if err := dr.Write(nestedPath, content); err != nil {
		t.Fatalf("Write(%q) failed: %v", nestedPath, err)
	}

	target := filepath.Join(tmpDir, "a", "b", "c", "file.txt")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("Expected file %s to exist, got %v", target, err)
	}
}
