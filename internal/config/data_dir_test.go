package config

import (
	"os"
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

func TestResolveDataDirHardenedExistingDirMode(t *testing.T) {
	base := t.TempDir()
	existing := filepath.Join(base, "insecure")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatalf("seed chmod: %v", err)
	}
	if _, err := ResolveDataDir(existing, true); err != nil {
		t.Fatalf("ResolveDataDir: %v", err)
	}
	info, err := os.Stat(existing)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("data dir perm=%#o want 0700", info.Mode().Perm())
	}
}
