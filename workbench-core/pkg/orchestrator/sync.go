package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// SyncRegistry aggregates child runs of an orchestrator run and writes registry/metrics
// into the orchestrator run's /agents directory.
func SyncRegistry(cfg config.Config, orchestratorRunID string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(orchestratorRunID) == "" {
		return fmt.Errorf("orchestratorRunID is required")
	}

	// Load orchestrator run to get session.
	orchRun, err := store.LoadRun(cfg, orchestratorRunID)
	if err != nil {
		return fmt.Errorf("load orchestrator run: %w", err)
	}

	sess, err := store.LoadSession(cfg, orchRun.SessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	reg := Registry{
		Version:           "v1",
		OrchestratorRunID: orchestratorRunID,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Tasks:             map[string]types.Task{},
		Agents:            map[string]AgentState{},
	}
	metrics := Metrics{Version: "v1", Tokens: TokenTotals{}, CostUSD: CostTotals{ByRun: map[string]float64{}}}

	for _, runID := range sess.Runs {
		run, err := store.LoadRun(cfg, runID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(run.ParentRunID) != strings.TrimSpace(orchestratorRunID) {
			continue
		}
		state := AgentState{
			RunID:         run.RunId,
			Status:        string(run.Status),
			CurrentTaskID: "",
			CurrentGoal:   strings.TrimSpace(run.Goal),
			Plan:          nil,
			SpawnedAt: func() string {
				if run.StartedAt != nil {
					return run.StartedAt.UTC().Format(time.RFC3339Nano)
				}
				return ""
			}(),
			LastPing: time.Now().UTC().Format(time.RFC3339Nano),
			Stats: AgentStats{
				TasksCompleted: 0,
				TasksFailed:    0,
				TokensIn:       run.TotalTokensIn,
				TokensOut:      run.TotalTokensOut,
				CostUSD:        run.TotalCostUSD,
			},
		}
		// Capture latest message/result (best-effort).
		items, _ := ReadOutbox(cfg, run.RunId)
		if len(items) > 0 {
			// pick last item
			last := items[len(items)-1]
			if last.Message != nil {
				state.LastMessage = last.Message
			}
		}
		reg.Agents[run.RunId] = state

		metrics.Tokens.In += run.TotalTokensIn
		metrics.Tokens.Out += run.TotalTokensOut
		metrics.CostUSD.Total += run.TotalCostUSD
		metrics.CostUSD.ByRun[run.RunId] = run.TotalCostUSD
	}

	// Persist to /agents in orchestrator run dir.
	agentsDir := filepath.Join(fsutil.GetRunDir(cfg.DataDir, orchestratorRunID), "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("prepare agents dir: %w", err)
	}
	if err := writeJSON(filepath.Join(agentsDir, "registry.json"), reg); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(agentsDir, "metrics.json"), metrics); err != nil {
		return err
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// EnqueueGoalTask enqueues a simple Task with goal into run inbox.
func EnqueueGoalTask(cfg config.Config, runID, goal string) (string, error) {
	return EnqueueTask(cfg, runID, types.Task{
		Goal:            goal,
		AssignedToRunID: runID,
	})
}

// LatestMessage returns the newest Message in the run's outbox (best-effort).
func LatestMessage(cfg config.Config, runID string) (*types.Message, error) {
	items, err := ReadOutbox(cfg, runID)
	if err != nil {
		return nil, err
	}
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Message != nil {
			return items[i].Message, nil
		}
	}
	return nil, nil
}

// ListChildRuns returns run IDs whose ParentRunID matches.
func ListChildRuns(cfg config.Config, sessionID, parentRunID string) ([]types.Run, error) {
	sess, err := store.LoadSession(cfg, sessionID)
	if err != nil {
		return nil, err
	}
	out := []types.Run{}
	for _, runID := range sess.Runs {
		run, err := store.LoadRun(cfg, runID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(run.ParentRunID) != strings.TrimSpace(parentRunID) {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(*out[j].StartedAt) })
	return out, nil
}
