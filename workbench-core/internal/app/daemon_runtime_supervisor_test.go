package app

import (
	"context"
	"sort"
	"sync"
	"testing"

	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestRuntimeSupervisorSyncOnce_StartsNonSystemRunsWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	ss := pkgstore.NewMemorySessionStore()

	systemSess := types.NewSession("system")
	systemSess.System = true
	systemSess.Runs = []string{"run-system"}
	systemSess.CurrentRunID = "run-system"
	if err := ss.SaveSession(ctx, systemSess); err != nil {
		t.Fatalf("save system session: %v", err)
	}

	userSess := types.NewSession("user")
	userSess.Runs = []string{"run-a", "run-b"}
	userSess.CurrentRunID = "run-a"
	if err := ss.SaveSession(ctx, userSess); err != nil {
		t.Fatalf("save user session: %v", err)
	}

	sup := newRuntimeSupervisor(runtimeSupervisorConfig{SessionStore: ss})
	var mu sync.Mutex
	started := map[string]int{}
	hold := make(chan struct{})
	sup.spawnOverride = func(_ context.Context, _ types.Session, runID string) (*managedRuntime, error) {
		mu.Lock()
		started[runID]++
		mu.Unlock()
		return &managedRuntime{cancel: func() {}, done: hold}, nil
	}

	if err := sup.syncOnce(ctx); err != nil {
		t.Fatalf("syncOnce #1: %v", err)
	}
	if err := sup.syncOnce(ctx); err != nil {
		t.Fatalf("syncOnce #2: %v", err)
	}

	mu.Lock()
	if started["run-system"] != 0 {
		t.Fatalf("expected system run to be skipped, got %d starts", started["run-system"])
	}
	if started["run-a"] != 1 || started["run-b"] != 1 {
		t.Fatalf("expected run-a/run-b each started once, got run-a=%d run-b=%d", started["run-a"], started["run-b"])
	}
	mu.Unlock()

	another := types.NewSession("another")
	another.Runs = []string{"run-c"}
	if err := ss.SaveSession(ctx, another); err != nil {
		t.Fatalf("save second user session: %v", err)
	}
	if err := sup.syncOnce(ctx); err != nil {
		t.Fatalf("syncOnce #3: %v", err)
	}
	mu.Lock()
	if started["run-c"] != 1 {
		t.Fatalf("expected run-c started once, got %d", started["run-c"])
	}
	mu.Unlock()
}

func TestCollectSessionRunIDs_DedupesAndPrefersCurrent(t *testing.T) {
	s := types.NewSession("x")
	s.CurrentRunID = "run-b"
	s.Runs = []string{"run-a", "run-b", "run-a", "", "run-c"}

	got := collectSessionRunIDs(s)
	if len(got) != 3 {
		t.Fatalf("expected 3 unique run IDs, got %v", got)
	}
	if got[0] != "run-b" {
		t.Fatalf("expected current run first, got %v", got)
	}
	rest := append([]string(nil), got[1:]...)
	sort.Strings(rest)
	if rest[0] != "run-a" || rest[1] != "run-c" {
		t.Fatalf("unexpected run IDs: %v", got)
	}
}

func TestRuntimeSupervisorSyncOnce_SkipsBootstrapRun(t *testing.T) {
	ctx := context.Background()
	ss := pkgstore.NewMemorySessionStore()

	userSess := types.NewSession("user")
	userSess.Runs = []string{"run-bootstrap", "run-other"}
	userSess.CurrentRunID = "run-bootstrap"
	if err := ss.SaveSession(ctx, userSess); err != nil {
		t.Fatalf("save user session: %v", err)
	}

	sup := newRuntimeSupervisor(runtimeSupervisorConfig{
		SessionStore:   ss,
		BootstrapRunID: "run-bootstrap",
	})
	started := map[string]int{}
	hold := make(chan struct{})
	sup.spawnOverride = func(_ context.Context, _ types.Session, runID string) (*managedRuntime, error) {
		started[runID]++
		return &managedRuntime{cancel: func() {}, done: hold}, nil
	}

	if err := sup.syncOnce(ctx); err != nil {
		t.Fatalf("syncOnce: %v", err)
	}
	if started["run-bootstrap"] != 0 {
		t.Fatalf("expected bootstrap run to be skipped, got %d", started["run-bootstrap"])
	}
	if started["run-other"] != 1 {
		t.Fatalf("expected run-other started once, got %d", started["run-other"])
	}
}
