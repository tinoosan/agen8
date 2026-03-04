package vfsutil

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoinBaseDir_BlocksEscapeAttempts(t *testing.T) {
	base := t.TempDir()

	tests := []string{
		"../x",
		"a/../../x",
		"a/b/../../../x",
		"..",
		"a/../..",
		"a/../../",
		"./../x",
	}

	for _, sub := range tests {
		t.Run(sub, func(t *testing.T) {
			_, err := SafeJoinBaseDir(base, sub)
			if err == nil {
				t.Fatalf("SafeJoinBaseDir(%q) should error", sub)
			}
			if !strings.Contains(err.Error(), "escapes mount root") {
				t.Fatalf("SafeJoinBaseDir(%q) error should mention escapes mount root, got: %v", sub, err)
			}
		})
	}
}

func TestSafeJoinBaseDir_BlocksAbsolutePaths(t *testing.T) {
	base := t.TempDir()
	tests := []string{
		"/etc/passwd",
		"/var/log/app.log",
	}
	for _, sub := range tests {
		t.Run(sub, func(t *testing.T) {
			_, err := SafeJoinBaseDir(base, sub)
			if err == nil {
				t.Fatalf("SafeJoinBaseDir(%q) should error", sub)
			}
			if !strings.Contains(err.Error(), "absolute paths not allowed") {
				t.Fatalf("SafeJoinBaseDir(%q) error should mention absolute paths not allowed, got: %v", sub, err)
			}
		})
	}
}

func TestSafeJoinBaseDir_AllowsNormalPaths(t *testing.T) {
	base := t.TempDir()
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("Abs(base): %v", err)
	}

	tests := []struct {
		name    string
		subpath string
		wantAbs string
	}{
		{name: "simple file", subpath: "notes.md", wantAbs: filepath.Join(baseAbs, "notes.md")},
		{name: "nested path", subpath: "reports/q1.md", wantAbs: filepath.Join(baseAbs, "reports", "q1.md")},
		{name: "normalization allowed", subpath: "a/../b/file.txt", wantAbs: filepath.Join(baseAbs, "b", "file.txt")},
		{name: "dot current directory", subpath: "./notes.md", wantAbs: filepath.Join(baseAbs, "notes.md")},
		{name: "empty path (root)", subpath: "", wantAbs: baseAbs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeJoinBaseDir(base, tt.subpath)
			if err != nil {
				t.Fatalf("SafeJoinBaseDir(%q) unexpected error: %v", tt.subpath, err)
			}
			if got != tt.wantAbs {
				t.Fatalf("SafeJoinBaseDir(%q) = %q, want %q", tt.subpath, got, tt.wantAbs)
			}
			if !strings.HasPrefix(got, baseAbs) {
				t.Fatalf("SafeJoinBaseDir(%q) result %q should start with baseAbs %q", tt.subpath, got, baseAbs)
			}
		})
	}
}

func TestRelUnderBaseDir(t *testing.T) {
	base := t.TempDir()
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("Abs(base): %v", err)
	}

	t.Run("contained path returns relative", func(t *testing.T) {
		target := filepath.Join(baseAbs, "a", "b", "c.txt")
		got, err := RelUnderBaseDir(baseAbs, target)
		if err != nil {
			t.Fatalf("RelUnderBaseDir error: %v", err)
		}
		if got != "a/b/c.txt" {
			t.Fatalf("RelUnderBaseDir = %q, want %q", got, "a/b/c.txt")
		}
	})

	t.Run("base itself returns dot", func(t *testing.T) {
		got, err := RelUnderBaseDir(baseAbs, baseAbs)
		if err != nil {
			t.Fatalf("RelUnderBaseDir error: %v", err)
		}
		if got != "." {
			t.Fatalf("RelUnderBaseDir = %q, want %q", got, ".")
		}
	})

	t.Run("outside path rejected", func(t *testing.T) {
		outside := filepath.Dir(baseAbs)
		_, err := RelUnderBaseDir(baseAbs, outside)
		if err == nil {
			t.Fatalf("RelUnderBaseDir should error for outside path")
		}
		if !strings.Contains(err.Error(), "escapes mount root") {
			t.Fatalf("RelUnderBaseDir error should mention escapes mount root, got: %v", err)
		}
	})
}

func FuzzSafeJoinBaseDir_RoundTripBehaviour(f *testing.F) {
	for _, seed := range []string{
		"",
		".",
		"file.txt",
		"a/b/c.md",
		"./nested/file",
		"../escape",
		"/abs/path",
		"a/../b",
		"a//b",
		" spaced/name.txt ",
	} {
		f.Add(seed)
	}

	base := f.TempDir()
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		f.Fatalf("Abs(base): %v", err)
	}

	f.Fuzz(func(t *testing.T, rawSubpath string) {
		if len(rawSubpath) > 2048 {
			t.Skip()
		}

		joined, err := SafeJoinBaseDir(baseAbs, rawSubpath)
		if err != nil {
			return
		}
		rel, err := RelUnderBaseDir(baseAbs, joined)
		if err != nil {
			t.Fatalf("RelUnderBaseDir(%q): %v", joined, err)
		}
		// Round-tripping through rel-under-base should preserve the absolute target.
		joined2, err := SafeJoinBaseDir(baseAbs, rel)
		if err != nil {
			t.Fatalf("SafeJoinBaseDir(roundtrip rel=%q): %v", rel, err)
		}
		if joined2 != joined {
			t.Fatalf("round-trip mismatch: raw=%q rel=%q joined=%q joined2=%q", rawSubpath, rel, joined, joined2)
		}

		relToBase, err := filepath.Rel(baseAbs, joined)
		if err != nil {
			t.Fatalf("filepath.Rel(%q,%q): %v", baseAbs, joined, err)
		}
		if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
			t.Fatalf("joined path escaped base: base=%q joined=%q rel=%q", baseAbs, joined, relToBase)
		}
	})
}
