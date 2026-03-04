package vfsutil

import (
	"errors"
	"path"
	"strings"
	"testing"
)

func TestNormalizeResourceSubpath(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantClean string
		wantParts []string
		wantErr   string
		wantErrIs []error
	}{
		{name: "empty is root", in: "", wantClean: "", wantParts: nil},
		{name: "dot is root", in: ".", wantClean: ".", wantParts: nil},
		{name: "simple path", in: "a/b", wantClean: "a/b", wantParts: []string{"a", "b"}},
		{name: "absolute rejected", in: "/etc/passwd", wantErr: "absolute paths not allowed", wantErrIs: []error{ErrInvalidPath}},
		{name: "escape rejected", in: "../x", wantErr: "escapes mount root", wantErrIs: []error{ErrInvalidPath, ErrEscapesRoot}},
		{name: "clean-away parent still rejected", in: "a/../x", wantErr: "escapes mount root", wantErrIs: []error{ErrInvalidPath, ErrEscapesRoot}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, parts, err := NormalizeResourceSubpath(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				for _, wantErr := range tt.wantErrIs {
					if !errors.Is(err, wantErr) {
						t.Fatalf("error = %v, want errors.Is(..., %v)", err, wantErr)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if clean != tt.wantClean {
				t.Fatalf("clean = %q, want %q", clean, tt.wantClean)
			}
			if len(parts) != len(tt.wantParts) {
				t.Fatalf("parts len = %d, want %d (parts=%v)", len(parts), len(tt.wantParts), parts)
			}
			for i := range parts {
				if parts[i] != tt.wantParts[i] {
					t.Fatalf("parts[%d] = %q, want %q (parts=%v)", i, parts[i], tt.wantParts[i], parts)
				}
			}
		})
	}
}

func TestCleanRelPath(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      string
		wantErr   string
		wantErrIs []error
	}{
		{name: "empty rejected", in: "", wantErr: "invalid path: empty"},
		{name: "absolute rejected", in: "/x", wantErr: "absolute paths not allowed", wantErrIs: []error{ErrInvalidPath}},
		{name: "escape rejected", in: "../x", wantErr: "escapes mount root", wantErrIs: []error{ErrInvalidPath, ErrEscapesRoot}},
		{name: "clean-away parent still rejected", in: "a/../x", wantErr: "escapes mount root", wantErrIs: []error{ErrInvalidPath, ErrEscapesRoot}},
		{name: "dot allowed", in: ".", want: "."},
		{name: "normal path", in: "a/b/c.txt", want: "a/b/c.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanRelPath(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				for _, wantErr := range tt.wantErrIs {
					if !errors.Is(err, wantErr) {
						t.Fatalf("error = %v, want errors.Is(..., %v)", err, wantErr)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("CleanRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanResultsArtifactPath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "empty rejected", in: "", wantErr: "artifact path is required"},
		{name: "absolute mapped", in: "/x", wantErr: "artifact path must be relative"},
		{name: "escape mapped", in: "../x", wantErr: "artifact path escapes results directory"},
		{name: "dot rejected", in: ".", wantErr: "artifact path is invalid"},
		{name: "normal path", in: "a/b.json", want: "a/b.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CleanResultsArtifactPath(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("CleanResultsArtifactPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzNormalizeResourceSubpath_Invariants(f *testing.F) {
	for _, seed := range []string{
		"",
		".",
		"a",
		"a/b",
		"a/../b",
		"../x",
		"/abs",
		"a//b",
		" a/b ",
		"./a",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 2048 {
			t.Skip()
		}
		clean, parts, err := NormalizeResourceSubpath(in)
		if err != nil {
			return
		}

		if clean == "" || clean == "." {
			if len(parts) != 0 {
				t.Fatalf("root-like clean path must have zero parts: clean=%q parts=%v", clean, parts)
			}
			return
		}

		if strings.HasPrefix(clean, "/") {
			t.Fatalf("normalized path must be relative: %q", clean)
		}
		if clean == ".." || strings.HasPrefix(clean, "../") {
			t.Fatalf("normalized path must not escape root: %q", clean)
		}
		if strings.Contains(clean, "//") {
			t.Fatalf("normalized path must not contain empty segments: %q", clean)
		}
		if path.Clean(clean) != clean {
			t.Fatalf("normalized path must already be clean: %q", clean)
		}
		if strings.Join(parts, "/") != clean {
			t.Fatalf("parts should reconstruct clean path: clean=%q parts=%v", clean, parts)
		}
		for _, p := range parts {
			if p == "" || p == "." || p == ".." {
				t.Fatalf("invalid normalized segment %q in %v", p, parts)
			}
		}
	})
}

func FuzzCleanResultsArtifactPath_Invariants(f *testing.F) {
	for _, seed := range []string{
		"",
		".",
		"result.json",
		"a/b/c.json",
		"../escape",
		"/abs/path",
		"a//b",
		" ./report.md ",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 2048 {
			t.Skip()
		}
		clean, err := CleanResultsArtifactPath(in)
		if err != nil {
			return
		}
		if clean == "" || clean == "." {
			t.Fatalf("artifact path must not be empty or dot: %q", clean)
		}
		if strings.HasPrefix(clean, "/") {
			t.Fatalf("artifact path must be relative: %q", clean)
		}
		if clean == ".." || strings.HasPrefix(clean, "../") {
			t.Fatalf("artifact path must not escape root: %q", clean)
		}
		if strings.Contains(clean, "//") {
			t.Fatalf("artifact path must not have empty segments: %q", clean)
		}
		if path.Clean(clean) != clean {
			t.Fatalf("artifact path must already be clean: %q", clean)
		}
	})
}
