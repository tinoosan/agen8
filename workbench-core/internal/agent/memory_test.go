package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
)

func TestLoadPersistentMemory_MissingFile_ReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	p := DefaultPersistentMemoryPath()
	got, err := LoadPersistentMemory(p)
	if err != nil {
		t.Fatalf("LoadPersistentMemory: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestAppendPersistentMemory_WritesAndAppends(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	p := DefaultPersistentMemoryPath()

	if err := AppendPersistentMemory(p, "first"); err != nil {
		t.Fatalf("AppendPersistentMemory: %v", err)
	}
	if err := AppendPersistentMemory(p, "second"); err != nil {
		t.Fatalf("AppendPersistentMemory: %v", err)
	}

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "first") || !strings.Contains(s, "second") {
		t.Fatalf("expected both updates in file, got:\n%s", s)
	}
	if _, err := os.Stat(filepath.Dir(p)); err != nil {
		t.Fatalf("expected directory to exist: %v", err)
	}
}

func TestBuildSystemPromptWithMemory(t *testing.T) {
	base := "base"
	mem := "note"
	out := BuildSystemPromptWithMemory(base, mem)
	if !strings.Contains(out, "Persistent Memory") {
		t.Fatalf("expected memory header, got %q", out)
	}
	if !strings.Contains(out, "note") {
		t.Fatalf("expected memory content, got %q", out)
	}
}
