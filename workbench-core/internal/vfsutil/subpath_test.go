package vfsutil

import (
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
	}{
		{name: "empty is root", in: "", wantClean: "", wantParts: nil},
		{name: "dot is root", in: ".", wantClean: ".", wantParts: nil},
		{name: "simple path", in: "a/b", wantClean: "a/b", wantParts: []string{"a", "b"}},
		{name: "absolute rejected", in: "/etc/passwd", wantErr: "absolute paths not allowed"},
		{name: "escape rejected", in: "../x", wantErr: "escapes mount root"},
		{name: "clean-away parent still rejected", in: "a/../x", wantErr: "escapes mount root"},
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
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "empty rejected", in: "", wantErr: "invalid path: empty"},
		{name: "absolute rejected", in: "/x", wantErr: "absolute paths not allowed"},
		{name: "escape rejected", in: "../x", wantErr: "escapes mount root"},
		{name: "clean-away parent still rejected", in: "a/../x", wantErr: "escapes mount root"},
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

