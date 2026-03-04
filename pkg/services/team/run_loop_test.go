package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
)

func TestRunRoleLoops_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var runCount int
	var mu sync.Mutex
	started := make(chan struct{}, 1)
	runner := &mockRoleRunner{
		run: func(ctx context.Context) error {
			mu.Lock()
			runCount++
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	runIDs := []string{"run-1"}
	var registered string
	registerCancel := func(runID string, c context.CancelFunc) {
		registered = runID
	}
	done := make(chan error, 1)
	go func() {
		done <- RunRoleLoops(ctx, []RoleRunner{runner}, runIDs, registerCancel)
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	err := <-done
	if err != nil && err != context.Canceled {
		t.Fatalf("RunRoleLoops: %v", err)
	}
	mu.Lock()
	n := runCount
	mu.Unlock()
	if n < 1 {
		t.Fatalf("runner should have been started at least once, got %d", n)
	}
	if registered != "run-1" {
		t.Fatalf("registerCancel called with %q", registered)
	}
}

func TestRunModelChangeLoop_ExitsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	taskStore := &mockTaskStore{countTasks: func(ctx context.Context, filter state.TaskFilter) (int, error) {
		return 1, nil
	}}
	store := &mockManifestStore{save: func(ctx context.Context, m Manifest) error { return nil }}
	stateMgr := NewStateManager(store, Manifest{TeamID: "team-1"})
	applier := &mockModelApplier{apply: func(ctx context.Context, model, target string) ([]string, error) {
		return nil, nil
	}}
	done := make(chan struct{})
	go func() {
		RunModelChangeLoop(ctx, taskStore, stateMgr, applier)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunModelChangeLoop did not exit on context cancel")
	}
}

type mockRoleRunner struct {
	run func(ctx context.Context) error
}

func (m *mockRoleRunner) Run(ctx context.Context) error {
	if m.run != nil {
		return m.run(ctx)
	}
	return nil
}
