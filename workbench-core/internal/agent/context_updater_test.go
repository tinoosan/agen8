package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/trace"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestContextUpdater_IncludesMemoryAndAdvancesTraceCursor(t *testing.T) {
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
	after1, err := trace.CursorToInt64(u.TraceCursor)
	if err != nil {
		t.Fatalf("CursorToInt64: %v", err)
	}
	if after1 == 0 {
		t.Fatalf("expected trace cursor to advance")
	}
	if m1.Memory.BytesIncluded == 0 {
		t.Fatalf("expected memory to be included")
	}
	before1, err := trace.CursorToInt64(m1.Trace.CursorBefore)
	if err != nil {
		t.Fatalf("CursorToInt64 before: %v", err)
	}
	afterManifest1, err := trace.CursorToInt64(m1.Trace.CursorAfter)
	if err != nil {
		t.Fatalf("CursorToInt64 after: %v", err)
	}
	if afterManifest1 <= before1 {
		t.Fatalf("expected trace cursor to advance in manifest")
	}
	if system1 == "base" {
		t.Fatalf("expected prompt to be augmented")
	}

	// Ensure manifest written.
	if _, err := fs.Read("/workspace/context_manifest.json"); err != nil {
		t.Fatalf("expected manifest in workspace: %v", err)
	}

	// Add another event and confirm offset advances again.
	before := u.TraceCursor
	if err := store.AppendEvent(run.RunId, "test.event2", "hello2", map[string]string{}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	_, m2, err := u.BuildSystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if u.TraceCursor == before {
		t.Fatalf("expected trace cursor to advance again")
	}
	if m2.Trace.CursorBefore != before {
		t.Fatalf("expected cursorBefore=%q got %q", before, m2.Trace.CursorBefore)
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

func TestContextUpdater_FiltersTraceEvents(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	run, err := store.CreateRun("updater filter test", 10)
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

	// Include one relevant event and one irrelevant event.
	if err := store.AppendEvent(run.RunId, "agent.op.request", "Agent requested host op", map[string]string{
		"op":   "fs.list",
		"path": "/",
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := store.AppendEvent(run.RunId, "irrelevant.event", "noise", map[string]string{
		"x": "y",
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if err := store.AppendEvent(run.RunId, "agent.op.response", "Host op completed", map[string]string{
		"op": "fs.list",
		"ok": "true",
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountMemory, memRes)
	fs.Mount(vfs.MountWorkspace, wsRes)

	u := &ContextUpdater{
		FS:             fs,
		MaxMemoryBytes: 1024,
		MaxTraceBytes:  2048,
	}
	system, m, err := u.BuildSystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if !strings.Contains(system, "## Recent Ops (from /trace)") {
		t.Fatalf("expected trace summary section, got:\n%s", system)
	}
	if strings.Contains(system, "irrelevant.event") {
		t.Fatalf("expected irrelevant event to be filtered out")
	}
	if m.Trace.Events.Selected == 0 {
		t.Fatalf("expected selected > 0")
	}
	if m.Trace.Events.Excluded == 0 {
		t.Fatalf("expected excluded > 0")
	}
}

func TestContextUpdater_AdaptiveBudgets(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	run, err := store.CreateRun("updater budget test", 10)
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

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountMemory, memRes)
	fs.Mount(vfs.MountWorkspace, wsRes)

	u := &ContextUpdater{
		FS:             fs,
		MaxMemoryBytes: 100,
		MaxTraceBytes:  200,
	}

	_, m1, err := u.BuildSystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("BuildSystemPrompt step1: %v", err)
	}
	if m1.Policy.Budgets.MemoryBytes != 100 {
		t.Fatalf("step1 memory budget=%d", m1.Policy.Budgets.MemoryBytes)
	}
	if m1.Policy.Budgets.TraceBytes != 400 {
		t.Fatalf("step1 trace budget=%d", m1.Policy.Budgets.TraceBytes)
	}

	_, m2, err := u.BuildSystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("BuildSystemPrompt step2: %v", err)
	}
	if m2.Policy.Budgets.MemoryBytes != 50 {
		t.Fatalf("step2 memory budget=%d", m2.Policy.Budgets.MemoryBytes)
	}
	if m2.Policy.Budgets.TraceBytes != 200 {
		t.Fatalf("step2 trace budget=%d", m2.Policy.Budgets.TraceBytes)
	}
}

func TestContextUpdater_FailureBumpAfterBadOp(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	run, err := store.CreateRun("updater failure bump test", 10)
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

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountMemory, memRes)
	fs.Mount(vfs.MountWorkspace, wsRes)

	u := &ContextUpdater{
		FS:             fs,
		MaxMemoryBytes: 100,
		MaxTraceBytes:  200,
	}

	u.ObserveHostOp(types.HostOpRequest{Op: "fs.read", Path: "/trace/events"}, types.HostOpResponse{
		Op:    "fs.read",
		Ok:    false,
		Error: "boom",
	})

	system, m, err := u.BuildSystemPrompt(context.Background(), "base", 2)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if !m.Policy.FailureBump {
		t.Fatalf("expected failure bump")
	}
	if m.Policy.Budgets.TraceBytes != 400 {
		t.Fatalf("expected bumped trace budget=400, got %d", m.Policy.Budgets.TraceBytes)
	}
	if !strings.Contains(system, "policy: failure bump active") {
		t.Fatalf("expected last op summary to include failure bump, got:\n%s", system)
	}
}
