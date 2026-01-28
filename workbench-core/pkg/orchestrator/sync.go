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
	childRuns := 0

	for _, runID := range sess.Runs {
		run, err := store.LoadRun(cfg, runID)
		if err != nil {
			continue
		}
		if strings.TrimSpace(run.ParentRunID) != strings.TrimSpace(orchestratorRunID) {
			continue
		}
		childRuns++
		tasks, _ := readInboxTasks(cfg, run.RunId)
		results, _ := ReadOutbox(cfg, run.RunId)

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
		// Derive status from tasks/results for fresher view.
		if len(tasks) > 0 {
			state.Status = "pending"
			for _, t := range tasks {
				if strings.EqualFold(strings.TrimSpace(t.Status), "in_progress") {
					state.Status = "busy"
					state.CurrentTaskID = t.TaskID
					state.CurrentGoal = strings.TrimSpace(t.Goal)
					break
				}
			}
			if state.CurrentTaskID == "" && len(tasks) > 0 {
				state.CurrentTaskID = tasks[0].TaskID
				state.CurrentGoal = strings.TrimSpace(tasks[0].Goal)
			}
		} else if len(results) > 0 {
			state.Status = "idle"
		}

		for _, t := range tasks {
			if t.TaskID == "" {
				continue
			}
			reg.Tasks[t.TaskID] = t
		}
		for _, item := range results {
			if item.TaskResult != nil {
				tr := item.TaskResult
				if tr.TaskID != "" {
					reg.Tasks[tr.TaskID] = types.Task{
						TaskID:      tr.TaskID,
						Status:      tr.Status,
						CompletedAt: tr.CompletedAt,
						Error:       tr.Error,
					}
				}
				if strings.EqualFold(tr.Status, "succeeded") {
					state.Stats.TasksCompleted++
				}
				if strings.EqualFold(tr.Status, "failed") {
					state.Stats.TasksFailed++
				}
			}
		}
		// Capture latest message/result (best-effort).
		if len(results) > 0 {
			// pick last item
			last := results[len(results)-1]
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
	// #region agent log
	debugLogSync("orchestrator/sync.go:148", "SyncRegistry summary", map[string]any{
		"sessionRuns":      len(sess.Runs),
		"childRuns":        childRuns,
		"agentCount":       len(reg.Agents),
		"taskCount":        len(reg.Tasks),
		"orchestratorRun":  orchestratorRunID,
		"orchestratorSess": orchRun.SessionID,
	})
	// #endregion
	return nil
}

func debugLogSync(location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": "H6",
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile("/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
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

// readInboxTasks reads JSON task envelopes from a run's inbox.
func readInboxTasks(cfg config.Config, runID string) ([]types.Task, error) {
	inboxDir := filepath.Join(fsutil.GetRunDir(cfg.DataDir, runID), "inbox")
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(inboxDir, e.Name()))
	}
	sort.Strings(paths)
	tasks := []types.Task{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var t types.Task
		if err := json.Unmarshal(b, &t); err != nil {
			continue
		}
		if strings.TrimSpace(t.TaskID) == "" {
			t.TaskID = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}
