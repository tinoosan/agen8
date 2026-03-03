package team

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"golang.org/x/sync/errgroup"
)

// RoleRunner runs one role's session loop (e.g. sess.Run). App wraps each teamRoleRuntime to implement this.
type RoleRunner interface {
	Run(ctx context.Context) error
}

// RunRoleLoops runs each runner in an errgroup with backoff on error, and registers each run's cancel
// so RunStopper.StopRun(runID) can cancel it. Returns when ctx is done or the errgroup returns.
func RunRoleLoops(
	ctx context.Context,
	runners []RoleRunner,
	runIDs []string,
	registerCancel func(runID string, cancel context.CancelFunc),
) error {
	if len(runners) != len(runIDs) {
		return nil
	}
	g, gctx := errgroup.WithContext(ctx)
	for i := range runners {
		runner := runners[i]
		runID := strings.TrimSpace(runIDs[i])
		if runID == "" {
			continue
		}
		g.Go(func() error {
			backoff := 2 * time.Second
			for {
				runLoopCtx, cancel := context.WithCancel(gctx)
				if registerCancel != nil {
					registerCancel(runID, cancel)
				}
				err := runner.Run(runLoopCtx)
				cancel()
				if gctx.Err() != nil {
					return nil
				}
				errMsg := "unknown error"
				if err != nil {
					errMsg = err.Error()
				}
				log.Printf("daemon: runner %s exited unexpectedly; restarting in %s: %s", runID, backoff, errMsg)
				select {
				case <-gctx.Done():
					return nil
				case <-time.After(backoff):
				}
				if backoff < 60*time.Second {
					backoff *= 2
					if backoff > 60*time.Second {
						backoff = 60 * time.Second
					}
				}
			}
		})
	}
	return g.Wait()
}

// RunModelChangeLoop runs a goroutine that every 5s checks for a pending model change and applies it when the team is idle.
func RunModelChangeLoop(ctx context.Context, taskStore state.TaskStore, stateMgr *StateManager, applier ModelApplier) {
	if stateMgr == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manifest := stateMgr.ManifestSnapshot()
			if manifest.ModelChange == nil {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(manifest.ModelChange.Status), "pending") {
				continue
			}
			if !IsTeamIdle(ctx, taskStore, manifest.TeamID) {
				continue
			}
			model := strings.TrimSpace(manifest.ModelChange.RequestedModel)
			if model == "" {
				continue
			}
			applied, err := applier.ApplyModel(ctx, model, "")
			if err != nil {
				_ = stateMgr.MarkModelFailed(ctx, model, err)
				log.Printf("daemon: apply queued team model failed: %v", err)
				continue
			}
			_ = stateMgr.MarkModelApplied(ctx, model)
			if len(applied) > 0 {
				log.Printf("daemon: applied queued team model: %s", model)
			}
		}
	}
}
