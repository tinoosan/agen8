package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestPromptBuilder_IncludesMemory(t *testing.T) {
	t.Parallel()

	fs := vfs.NewFS()
	memDir := t.TempDir()

	memRes, err := resources.NewDirResource(memDir, vfs.MountMemory)
	if err != nil {
		t.Fatalf("NewDirResource(memory): %v", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		t.Fatalf("mount memory: %v", err)
	}

	today := time.Now().Format("2006-01-02") + "-memory.md"
	if err := os.WriteFile(filepath.Join(memDir, today), []byte("remember this"), 0644); err != nil {
		t.Fatalf("write daily memory: %v", err)
	}

	constructor := &PromptBuilder{
		FS:              fs,
		MaxMemoryBytes:  1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "## Memory") || !strings.Contains(out, "remember this") {
		t.Fatalf("expected memory section, got: %q", out)
	}
}

func TestPromptBuilder_OmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	constructor := &PromptBuilder{
		FS:              vfs.NewFS(),
		MaxMemoryBytes:  1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "## User Profile") || strings.Contains(out, "## Memory") {
		t.Fatalf("did not expect memory section, got: %q", out)
	}
}
