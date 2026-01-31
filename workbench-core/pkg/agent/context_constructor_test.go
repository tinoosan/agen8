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

func TestContextConstructor_IncludesProfileAndMemory(t *testing.T) {
	t.Parallel()

	fs := vfs.NewFS()
	memDir := t.TempDir()
	profileDir := t.TempDir()

	memRes, err := resources.NewDirResource(memDir, vfs.MountMemory)
	if err != nil {
		t.Fatalf("NewDirResource(memory): %v", err)
	}
	profileRes, err := resources.NewDirResource(profileDir, vfs.MountProfile)
	if err != nil {
		t.Fatalf("NewDirResource(profile): %v", err)
	}
	fs.Mount(vfs.MountMemory, memRes)
	fs.Mount(vfs.MountProfile, profileRes)

	today := time.Now().Format("2006-01-02") + "-memory.md"
	if err := os.WriteFile(filepath.Join(memDir, today), []byte("remember this"), 0644); err != nil {
		t.Fatalf("write daily memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.md"), []byte("profile info"), 0644); err != nil {
		t.Fatalf("write profile.md: %v", err)
	}

	constructor := &ContextConstructor{
		FS:              fs,
		MaxProfileBytes: 1024,
		MaxMemoryBytes:  1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "## Profile") || !strings.Contains(out, "profile info") {
		t.Fatalf("expected profile section, got: %q", out)
	}
	if !strings.Contains(out, "## Memory") || !strings.Contains(out, "remember this") {
		t.Fatalf("expected memory section, got: %q", out)
	}
}

func TestContextConstructor_OmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	constructor := &ContextConstructor{
		FS:              vfs.NewFS(),
		MaxProfileBytes: 1024,
		MaxMemoryBytes:  1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "## Profile") || strings.Contains(out, "## Memory") {
		t.Fatalf("did not expect profile/memory sections, got: %q", out)
	}
}
