package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func buildTeamRuntimeForTest(t *testing.T, dataDir string, run types.Run, teamID string) *Runtime {
	t.Helper()

	cfg := config.Config{DataDir: dataDir}
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

	rt, err := Build(BuildConfig{
		Cfg:                cfg,
		Run:                run,
		Profile:            "team-profile",
		WorkdirAbs:         t.TempDir(),
		SharedWorkspaceDir: fsutil.GetTeamWorkspaceDir(dataDir, teamID),
		Model:              "test-model",
		ReasoningEffort:    "minimal",
		ReasoningSummary:   "auto",
		ApprovalsMode:      "disabled",
		HistoryStore:       historyStore,
		MemoryStore:        memoryStore,
		TraceStore:         implstore.SQLiteTraceStore{Cfg: cfg, RunID: run.RunID},
		ConstructorStore:   constructorStore,
		IncludeHistoryOps:  true,
	})
	if err != nil {
		t.Fatalf("runtime.Build: %v", err)
	}
	t.Cleanup(func() {
		_ = rt.Shutdown(context.Background())
	})
	return rt
}

func TestBuild_TeamSharedWorkspaceNoPrecreatedRoleOrDeliverables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	run := types.NewRun("team task", 8*1024, "sess-team-1")
	run.Runtime = &types.RunRuntimeConfig{
		TeamID: "team-1",
		Role:   "backend-engineer",
	}

	_ = buildTeamRuntimeForTest(t, dataDir, run, "team-1")
	teamWorkspace := fsutil.GetTeamWorkspaceDir(dataDir, "team-1")

	if _, err := os.Stat(filepath.Join(teamWorkspace, "backend-engineer")); !os.IsNotExist(err) {
		t.Fatalf("expected no precreated role workspace directory, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(teamWorkspace, "deliverables")); !os.IsNotExist(err) {
		t.Fatalf("expected no precreated team deliverables directory, got err=%v", err)
	}
}

func TestBuild_TeamSharedTasksRootNoPrecreatedRoleTasks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	run := types.NewRun("team task", 8*1024, "sess-team-1")
	run.Runtime = &types.RunRuntimeConfig{
		TeamID: "team-1",
		Role:   "backend-engineer",
	}

	_ = buildTeamRuntimeForTest(t, dataDir, run, "team-1")
	teamTasks := fsutil.GetTeamTasksDir(dataDir, "team-1")
	if st, err := os.Stat(teamTasks); err != nil || !st.IsDir() {
		t.Fatalf("expected team tasks root directory at %q, err=%v", teamTasks, err)
	}
	if _, err := os.Stat(filepath.Join(teamTasks, "backend-engineer")); !os.IsNotExist(err) {
		t.Fatalf("expected no precreated role tasks directory, got err=%v", err)
	}
}

func TestBuild_TeamContextManifestWritesToRunDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dataDir := t.TempDir()
	run := types.NewRun("team task", 8*1024, "sess-team-1")
	run.Runtime = &types.RunRuntimeConfig{
		TeamID: "team-1",
		Role:   "backend-engineer",
	}

	rt := buildTeamRuntimeForTest(t, dataDir, run, "team-1")
	if rt.Updater == nil {
		t.Fatalf("expected prompt updater")
	}
	if _, _, err := rt.Updater.BuildSystemPrompt(context.Background(), "base", 1); err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}

	runManifestPath := filepath.Join(fsutil.GetRunDir(dataDir, run), "context_constructor.json")
	if _, err := os.Stat(runManifestPath); err != nil {
		t.Fatalf("expected manifest at run dir path %q: %v", runManifestPath, err)
	}

	workspaceManifestPath := filepath.Join(fsutil.GetTeamWorkspaceDir(dataDir, "team-1"), "context_constructor_manifest.json")
	if _, err := os.Stat(workspaceManifestPath); !os.IsNotExist(err) {
		t.Fatalf("expected no workspace manifest at %q, got err=%v", workspaceManifestPath, err)
	}
}
