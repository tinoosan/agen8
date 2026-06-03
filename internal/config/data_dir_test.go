package config

import (
	"path/filepath"
	"testing"
)

func TestResolveDataDirReturnsAbsolutePathForRelativeCLIValue(t *testing.T) {
	dir, err := ResolveDataDir(filepath.Join(".", "tmp-test-data-dir"), true)
	if err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("ResolveDataDir returned %q, want absolute path", dir)
	}
}

func TestDefaultDataDirForDarwin(t *testing.T) {
	dir, err := defaultDataDirFor("darwin", "/Users/santino", "/tmp/xdg-state", "/tmp/config")
	if err != nil {
		t.Fatalf("defaultDataDirFor: %v", err)
	}
	want := filepath.Join("/Users/santino", ".agen8")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestDefaultDataDirForLinuxUsesXDGState(t *testing.T) {
	dir, err := defaultDataDirFor("linux", "/home/santino", "/run/user/1000/state", "")
	if err != nil {
		t.Fatalf("defaultDataDirFor: %v", err)
	}
	want := filepath.Join("/run/user/1000/state", "agen8")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestDefaultDataDirForLinuxUsesLocalStateFallback(t *testing.T) {
	dir, err := defaultDataDirFor("linux", "/home/santino", "", "")
	if err != nil {
		t.Fatalf("defaultDataDirFor: %v", err)
	}
	want := filepath.Join("/home/santino", ".local", "state", "agen8")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}

func TestDefaultDataDirForWindowsUsesUserConfigDir(t *testing.T) {
	dir, err := defaultDataDirFor("windows", `C:\Users\santino`, "", `C:\Users\santino\AppData\Roaming`)
	if err != nil {
		t.Fatalf("defaultDataDirFor: %v", err)
	}
	want := filepath.Join(`C:\Users\santino\AppData\Roaming`, "agen8")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
}
