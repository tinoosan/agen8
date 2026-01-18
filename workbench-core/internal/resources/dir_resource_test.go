package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirResource_TraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{BaseDir: tmpDir, Mount: "workspace"}

	// Avoid duplicating the full path matrix here; those cases live under vfsutil tests.
	if err := dr.Write("../x", []byte("nope")); err == nil || !strings.Contains(err.Error(), "escapes mount root") {
		t.Fatalf("Write traversal should be rejected with escape error, got: %v", err)
	}
	if _, err := dr.Read("/etc/passwd"); err == nil || !strings.Contains(err.Error(), "absolute paths not allowed") {
		t.Fatalf("Read absolute path should be rejected with absolute-path error, got: %v", err)
	}
}

// TestWriteRead_Roundtrip verifies that data written to a file can be read back correctly.
func TestWriteRead_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	content := []byte("hi")
	filename := "notes.md"

	// Write
	err := dr.Write(filename, content)
	if err != nil {
		t.Fatalf("Write(%q) failed: %v", filename, err)
	}

	// Read
	readContent, err := dr.Read(filename)
	if err != nil {
		t.Fatalf("Read(%q) failed: %v", filename, err)
	}

	// Verify
	if string(readContent) != string(content) {
		t.Errorf("Read(%q) = %q, want %q", filename, readContent, content)
	}
}

// TestAppend_CreatesFile verifies that Append creates a file if it doesn't exist
// and properly appends data to existing files.
func TestAppend_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	filename := "log.txt"
	firstLine := []byte("a\n")
	secondLine := []byte("b\n")

	// First append - should create file
	err := dr.Append(filename, firstLine)
	if err != nil {
		t.Fatalf("First Append(%q) failed: %v", filename, err)
	}

	// Second append - should add to existing file
	err = dr.Append(filename, secondLine)
	if err != nil {
		t.Fatalf("Second Append(%q) failed: %v", filename, err)
	}

	// Read and verify both lines are present
	content, err := dr.Read(filename)
	if err != nil {
		t.Fatalf("Read(%q) failed: %v", filename, err)
	}

	expected := "a\nb\n"
	if string(content) != expected {
		t.Errorf("Read(%q) = %q, want %q", filename, content, expected)
	}
}

// TestList_ReturnsCorrectVFSPaths verifies that List returns entries with proper VFS paths.
// TestList_ReturnsCorrectPaths verifies that List returns entries with proper relative paths.
func TestList_ReturnsCorrectPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	// Create test files and directories
	if err := dr.Write("notes.md", []byte("test content")); err != nil {
		t.Fatalf("Failed to create notes.md: %v", err)
	}

	if err := dr.Write("reports/q1.md", []byte("Q1 report")); err != nil {
		t.Fatalf("Failed to create reports/q1.md: %v", err)
	}

	// Test listing root directory
	t.Run("list root", func(t *testing.T) {
		entries, err := dr.List("")
		if err != nil {
			t.Fatalf("List(\"\") failed: %v", err)
		}

		// Should contain both notes.md and reports directory
		if len(entries) != 2 {
			t.Errorf("List(\"\") returned %d entries, want 2", len(entries))
		}

		// Check for notes.md and reports (paths are now relative, not full VFS paths)
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

	// Test listing subdirectory
	t.Run("list reports subdirectory", func(t *testing.T) {
		entries, err := dr.List("reports")
		if err != nil {
			t.Fatalf("List(\"reports\") failed: %v", err)
		}

		// Should contain q1.md
		if len(entries) != 1 {
			t.Errorf("List(\"reports\") returned %d entries, want 1", len(entries))
		}

		// Path is now relative to the subpath
		if entries[0].Path != "reports/q1.md" {
			t.Errorf("List(\"reports\") entry path = %q, want %q", entries[0].Path, "reports/q1.md")
		}

		if entries[0].IsDir {
			t.Error("List(\"reports\") q1.md should not be marked as directory")
		}
	})

	// Test that file entries have size and modtime
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
				// Size should be non-negative (empty files have size 0, which is valid)
				if e.Size < 0 {
					t.Errorf("File entry %q should have Size >= 0, got %d", e.Path, e.Size)
				}
			}
		}
	})
}

// TestWrite_CreatesParentDirectories verifies that Write creates parent directories as needed.
func TestWrite_CreatesParentDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	// Write to a nested path that doesn't exist
	nestedPath := "a/b/c/file.txt"
	content := []byte("nested content")

	err := dr.Write(nestedPath, content)
	if err != nil {
		t.Fatalf("Write(%q) failed: %v", nestedPath, err)
	}

	// Verify the file exists
	fullPath := filepath.Join(tmpDir, "a", "b", "c", "file.txt")
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("Write(%q) should have created parent directories", nestedPath)
	}

	// Verify content
	readContent, err := dr.Read(nestedPath)
	if err != nil {
		t.Fatalf("Read(%q) failed: %v", nestedPath, err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Read(%q) = %q, want %q", nestedPath, readContent, content)
	}
}
