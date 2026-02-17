package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestBuild_StandaloneSubagentMountsParentIndexedRoots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	cfg := config.Config{DataDir: dataDir}
	run := types.NewRun("child task", 8*1024, "sess-standalone-1")
	run.RunID = "run-child"
	run.ParentRunID = "run-parent"
	run.SpawnIndex = 2

	historyStore, err := implstore.NewSQLiteHistoryStore(cfg, run.SessionID)
	if err != nil {
		t.Fatalf("NewSQLiteHistoryStore: %v", err)
	}
	memoryStore, err := implstore.NewDiskMemoryStore(cfg)
	if err != nil {
		t.Fatalf("NewDiskMemoryStore: %v", err)
	}
	constructorStore, err := implstore.NewSQLiteConstructorStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteConstructorStore: %v", err)
	}

	var mounted map[string]string
	rt, err := Build(BuildConfig{
		Cfg:              cfg,
		Run:              run,
		Profile:          "standalone-profile",
		WorkdirAbs:       t.TempDir(),
		Model:            "test-model",
		ReasoningEffort:  "minimal",
		ReasoningSummary: "auto",
		ApprovalsMode:    "disabled",
		HistoryStore:     historyStore,
		MemoryStore:      memoryStore,
		TraceStore:       implstore.SQLiteTraceStore{Cfg: cfg, RunID: run.RunID},
		ConstructorStore: constructorStore,
		IncludeHistoryOps: true,
		Emit: func(_ context.Context, ev events.Event) {
			if ev.Type == "host.mounted" {
				mounted = ev.Data
			}
		},
	})
	if err != nil {
		t.Fatalf("runtime.Build: %v", err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	wantWorkspace := fsutil.GetStandaloneSubagentWorkspaceDir(dataDir, run.ParentRunID, run.SpawnIndex)
	wantTasks := fsutil.GetStandaloneSubagentTasksDir(dataDir, run.ParentRunID, run.SpawnIndex)
	wantPlan := fsutil.GetStandaloneSubagentPlanDir(dataDir, run.ParentRunID, run.SpawnIndex)

	if st, err := os.Stat(wantWorkspace); err != nil || !st.IsDir() {
		t.Fatalf("expected standalone subagent workspace at %q, err=%v", wantWorkspace, err)
	}
	if st, err := os.Stat(wantTasks); err != nil || !st.IsDir() {
		t.Fatalf("expected standalone subagent tasks at %q, err=%v", wantTasks, err)
	}
	if st, err := os.Stat(wantPlan); err != nil || !st.IsDir() {
		t.Fatalf("expected standalone subagent plan at %q, err=%v", wantPlan, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agents", run.ParentRunID, "deliverables")); !os.IsNotExist(err) {
		t.Fatalf("expected no parent deliverables root for standalone subagent flow, got err=%v", err)
	}

	if got := mounted["/workspace"]; got != wantWorkspace {
		t.Fatalf("mounted /workspace = %q, want %q", got, wantWorkspace)
	}
	if got := mounted["/tasks"]; got != wantTasks {
		t.Fatalf("mounted /tasks = %q, want %q", got, wantTasks)
	}
	if got := mounted["/plan"]; got != wantPlan {
		t.Fatalf("mounted /plan = %q, want %q", got, wantPlan)
	}
	if _, ok := mounted["/deliverables"]; ok {
		t.Fatalf("did not expect /deliverables mount in standalone subagent mode")
	}
}

func TestBuild_StandaloneTopLevel_NoDeliverablesMount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	cfg := config.Config{DataDir: dataDir}
	run := types.NewRun("top-level task", 8*1024, "sess-standalone-2")

	historyStore, err := implstore.NewSQLiteHistoryStore(cfg, run.SessionID)
	if err != nil {
		t.Fatalf("NewSQLiteHistoryStore: %v", err)
	}
	memoryStore, err := implstore.NewDiskMemoryStore(cfg)
	if err != nil {
		t.Fatalf("NewDiskMemoryStore: %v", err)
	}
	constructorStore, err := implstore.NewSQLiteConstructorStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteConstructorStore: %v", err)
	}

	var mounted map[string]string
	rt, err := Build(BuildConfig{
		Cfg:               cfg,
		Run:               run,
		Profile:           "standalone-profile",
		WorkdirAbs:        t.TempDir(),
		Model:             "test-model",
		ReasoningEffort:   "minimal",
		ReasoningSummary:  "auto",
		ApprovalsMode:     "disabled",
		HistoryStore:      historyStore,
		MemoryStore:       memoryStore,
		TraceStore:        implstore.SQLiteTraceStore{Cfg: cfg, RunID: run.RunID},
		ConstructorStore:  constructorStore,
		IncludeHistoryOps: true,
		Emit: func(_ context.Context, ev events.Event) {
			if ev.Type == "host.mounted" {
				mounted = ev.Data
			}
		},
	})
	if err != nil {
		t.Fatalf("runtime.Build: %v", err)
	}
	defer func() { _ = rt.Shutdown(context.Background()) }()

	if _, ok := mounted["/deliverables"]; ok {
		t.Fatalf("did not expect /deliverables mount in standalone top-level mode")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agents", run.RunID, "deliverables")); !os.IsNotExist(err) {
		t.Fatalf("expected no standalone run deliverables directory, got err=%v", err)
	}
}
