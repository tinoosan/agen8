package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeJoin_BlocksEscapeAttempts verifies that safeJoin properly rejects
// paths that attempt to escape the base directory.
func TestSafeJoin_BlocksEscapeAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	tests := []struct {
		name    string
		subpath string
	}{
		{
			name:    "direct parent reference",
			subpath: "../x",
		},
		{
			name:    "multiple parent references",
			subpath: "a/../../x",
		},
		{
			name:    "triple parent reference",
			subpath: "a/b/../../../x",
		},
		{
			name:    "just parent directory",
			subpath: "..",
		},
		{
			name:    "tricky double parent",
			subpath: "a/../..",
		},
		{
			name:    "tricky with trailing separator",
			subpath: "a/../../",
		},
		{
			name:    "leading dot parent",
			subpath: "./../x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dr.safeJoin(tt.subpath)
			if err == nil {
				t.Fatalf("safeJoin(%q) should have returned an error for escape attempt", tt.subpath)
			}
			// Check for expected error messages
			msg := err.Error()
			if !strings.Contains(msg, "escapes mount root") {
				t.Errorf("safeJoin(%q) error should mention 'escapes mount root', got: %v", tt.subpath, err)
			}
		})
	}
}

// TestSafeJoin_BlocksAbsolutePaths verifies that safeJoin rejects absolute paths.
func TestSafeJoin_BlocksAbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	tests := []struct {
		name    string
		subpath string
	}{
		{
			name:    "unix absolute path",
			subpath: "/etc/passwd",
		},
		{
			name:    "unix absolute with subdirs",
			subpath: "/var/log/app.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dr.safeJoin(tt.subpath)
			if err == nil {
				t.Fatalf("safeJoin(%q) should have returned an error for absolute path", tt.subpath)
			}
			msg := err.Error()
			if !strings.Contains(msg, "absolute paths not allowed") {
				t.Errorf("safeJoin(%q) error should mention 'absolute paths not allowed', got: %v", tt.subpath, err)
			}
		})
	}
}

// TestSafeJoin_AllowsNormalPaths verifies that safeJoin accepts valid relative paths.
func TestSafeJoin_AllowsNormalPaths(t *testing.T) {
	tmpDir := t.TempDir()
	dr := &DirResource{
		BaseDir: tmpDir,
		Mount:   "workspace",
	}

	tests := []struct {
		name     string
		subpath  string
		expected string // expected suffix of the result
	}{
		{
			name:     "simple file",
			subpath:  "notes.md",
			expected: "notes.md",
		},
		{
			name:     "nested path",
			subpath:  "reports/q1.md",
			expected: filepath.Join("reports", "q1.md"),
		},
		{
			name:     "normalization allowed",
			subpath:  "a/../b/file.txt",
			expected: filepath.Join("b", "file.txt"),
		},
		{
			name:     "dot current directory",
			subpath:  "./notes.md",
			expected: "notes.md",
		},
		{
			name:     "empty path (root)",
			subpath:  "",
			expected: "", // should resolve to tmpDir itself
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := dr.safeJoin(tt.subpath)
			if err != nil {
				t.Fatalf("safeJoin(%q) should not return error for valid path, got: %v", tt.subpath, err)
			}

			// Result should start with tmpDir
			if !strings.HasPrefix(result, tmpDir) {
				t.Errorf("safeJoin(%q) result %q should start with baseDir %q", tt.subpath, result, tmpDir)
			}

			// Result should not contain ".."
			if strings.Contains(result, "..") {
				t.Errorf("safeJoin(%q) result %q should not contain '..'", tt.subpath, result)
			}

			// Check expected suffix if provided
			if tt.expected != "" {
				expected := filepath.Join(tmpDir, tt.expected)
				if result != expected {
					t.Errorf("safeJoin(%q) = %q, want %q", tt.subpath, result, expected)
				}
			}
		})
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
func TestList_ReturnsCorrectVFSPaths(t *testing.T) {
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

		// Check for notes.md
		foundNotes := false
		foundReports := false
		for _, e := range entries {
			if e.Path == "/workspace/notes.md" && !e.IsDir {
				foundNotes = true
			}
			if e.Path == "/workspace/reports" && e.IsDir {
				foundReports = true
			}
		}

		if !foundNotes {
			t.Error("List(\"\") should contain /workspace/notes.md")
		}
		if !foundReports {
			t.Error("List(\"\") should contain /workspace/reports directory")
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

		if entries[0].Path != "/workspace/reports/q1.md" {
			t.Errorf("List(\"reports\") entry path = %q, want %q", entries[0].Path, "/workspace/reports/q1.md")
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
