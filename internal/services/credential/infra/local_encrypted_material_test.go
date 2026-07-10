package infra

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalDataKeyGeneratesAndReusesKey(t *testing.T) {
	dataDir := t.TempDir()

	first, err := localDataKey(dataDir)
	if err != nil {
		t.Fatalf("localDataKey first: %v", err)
	}
	if len(first) != keySize {
		t.Fatalf("key length=%d want %d", len(first), keySize)
	}
	keyPath := filepath.Join(dataDir, keyDirName, keyFileName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("key permissions=%#o want 0600", mode)
	}

	second, err := localDataKey(dataDir)
	if err != nil {
		t.Fatalf("localDataKey second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("localDataKey did not reuse existing key")
	}
}

func TestLocalDataKeyRejectsSymlinkedKeyDir(t *testing.T) {
	dataDir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dataDir, keyDirName)); err != nil {
		t.Fatalf("symlink key dir: %v", err)
	}

	_, err := localDataKey(dataDir)
	if err == nil {
		t.Fatal("localDataKey should reject symlinked key directory")
	}
	if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("unexpected error=%v", err)
	}
}

func TestLocalDataKeyRejectsSymlinkedKeyFile(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, keyDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	outside := filepath.Join(t.TempDir(), keyFileName)
	if err := os.WriteFile(outside, bytes.Repeat([]byte{1}, keySize), 0o600); err != nil {
		t.Fatalf("write outside key: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, keyFileName)); err != nil {
		t.Fatalf("symlink key file: %v", err)
	}

	_, err := localDataKey(dataDir)
	if err == nil {
		t.Fatal("localDataKey should reject symlinked key file")
	}
	if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("unexpected error=%v", err)
	}
}
