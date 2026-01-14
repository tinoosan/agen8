package agent

import (
	"context"
	"os"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestContextUpdater_IncludesMemoryAndAdvancesTraceOffset(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	run, err := store.CreateRun("updater test", 10)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	traceRes, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		t.Fatalf("NewTraceResource: %v", err)
	}
	memRes, err := resources.NewRunMemoryResource(run.RunId)
	if err != nil {
		t.Fatalf("NewRunMemoryResource: %v", err)
	}
	wsRes, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		t.Fatalf("NewRunWorkspace: %v", err)
	}

	// Seed run-scoped memory.md on disk (read-only to the agent).
	memPath := fsutil.GetRunMemoryPath(config.DataDir, run.RunId)
	if err := os.WriteFile(memPath, []byte("remember this"), 0644); err != nil {
		t.Fatalf("WriteFile memory.md: %v", err)
	}

	// Seed trace with one event.
	if err := store.AppendEvent(run.RunId, "test.event", "hello", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountMemory, memRes)
	fs.Mount(vfs.MountWorkspace, wsRes)

	u := &ContextUpdater{
		FS:             fs,
		MaxMemoryBytes: 1024,
		MaxTraceBytes:  4096,
		ManifestPath:   "/workspace/context_manifest.json",
	}

	system1, m1, err := u.BuildSystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if u.TraceOffset == 0 {
		t.Fatalf("expected trace offset to advance")
	}
	if m1.Memory.BytesIncluded == 0 {
		t.Fatalf("expected memory to be included")
	}
	if m1.Trace.OffsetAfter <= m1.Trace.OffsetBefore {
		t.Fatalf("expected trace offset to advance in manifest")
	}
	if system1 == "base" {
		t.Fatalf("expected prompt to be augmented")
	}

	// Ensure manifest written.
	if _, err := fs.Read("/workspace/context_manifest.json"); err != nil {
		t.Fatalf("expected manifest in workspace: %v", err)
	}

	// Add another event and confirm offset advances again.
	before := u.TraceOffset
	if err := store.AppendEvent(run.RunId, "test.event2", "hello2", map[string]string{}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	_, m2, err := u.BuildSystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if u.TraceOffset <= before {
		t.Fatalf("expected trace offset to advance again")
	}
	if m2.Trace.OffsetBefore != before {
		t.Fatalf("expected offsetBefore=%d got %d", before, m2.Trace.OffsetBefore)
	}
}

func TestHeadTailUTF8(t *testing.T) {
	// Invalid utf8 prefix/suffix trimming should not panic and should return valid UTF-8.
	b := []byte{0xff, 'h', 'i', 0xff}
	h, _ := headUTF8(b, 3)
	if len(h) != 2 {
		t.Fatalf("unexpected head len %d", len(h))
	}
	tail, _ := tailUTF8(b, 3)
	if len(tail) != 2 {
		t.Fatalf("unexpected tail len %d", len(tail))
	}
}
