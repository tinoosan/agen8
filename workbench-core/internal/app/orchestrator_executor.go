package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/orchestrator"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type orchestratorExecutor struct {
	base   agent.HostExecutor
	runner *tuiTurnRunner
}

func (x *orchestratorExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	switch req.Op {
	case types.HostOpOrchestratorSpawn:
		if x.runner != nil {
			x.runner.markOrchestratorToolCall()
		}
		return x.handleSpawn(ctx, req)
	case types.HostOpOrchestratorTask:
		if x.runner != nil {
			x.runner.markOrchestratorToolCall()
		}
		return x.handleTask(ctx, req)
	case types.HostOpOrchestratorMessage:
		if x.runner != nil {
			x.runner.markOrchestratorToolCall()
		}
		return x.handleMessage(ctx, req)
	case types.HostOpOrchestratorSync:
		if x.runner != nil {
			x.runner.markOrchestratorToolCall()
		}
		return x.handleSync(ctx)
	case types.HostOpOrchestratorList:
		if x.runner != nil {
			x.runner.markOrchestratorToolCall()
		}
		return x.handleList(ctx)
	default:
		if x.base == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "base executor missing"}
		}
		return x.base.Exec(ctx, req)
	}
}

type spawnInput struct {
	Goal     string         `json:"goal"`
	Priority string         `json:"priority,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (x *orchestratorExecutor) handleSpawn(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x.runner == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "runner missing"}
	}
	var in spawnInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	goal := strings.TrimSpace(in.Goal)
	if goal == "" {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "goal is required"}
	}
	run, err := store.CreateSubRun(x.runner.cfg, x.runner.run.SessionID, x.runner.run.RunId, goal, x.runner.run.MaxBytesForContext)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	if err := x.runner.startSwarmWorker(run); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	_, _ = orchestrator.EnqueueTask(x.runner.cfg, run.RunId, types.Task{
		Goal:            goal,
		AssignedToRunID: run.RunId,
		Priority:        strings.TrimSpace(in.Priority),
		Metadata:        in.Metadata,
	})
	syncErr := orchestrator.SyncRegistry(x.runner.cfg, x.runner.run.RunId)
	if reg, err := orchestrator.LoadRegistryFile(x.runner.cfg, x.runner.run.RunId); err == nil {
		x.runner.setSwarmSummary(renderSwarmSummary(reg))
	}
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: run.RunId, Error: errString(syncErr)}
}

type taskInput struct {
	RunID    string         `json:"runId"`
	Goal     string         `json:"goal"`
	WaitFor  []string       `json:"waitFor,omitempty"`
	Priority string         `json:"priority,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (x *orchestratorExecutor) handleTask(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x.runner == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "runner missing"}
	}
	var in taskInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	runID := strings.TrimSpace(in.RunID)
	goal := strings.TrimSpace(in.Goal)
	if runID == "" || goal == "" {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "runId and goal are required"}
	}
	if run, err := store.LoadRun(x.runner.cfg, runID); err == nil {
		_ = x.runner.startSwarmWorker(run)
	}
	task := types.Task{
		TaskID:          "task-" + uuid.NewString(),
		AssignedToRunID: runID,
		Goal:            goal,
		WaitFor:         in.WaitFor,
		Priority:        strings.TrimSpace(in.Priority),
		Inputs:          in.Inputs,
		Metadata:        in.Metadata,
	}
	if _, err := orchestrator.EnqueueTask(x.runner.cfg, runID, task); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	syncErr := orchestrator.SyncRegistry(x.runner.cfg, x.runner.run.RunId)
	if reg, err := orchestrator.LoadRegistryFile(x.runner.cfg, x.runner.run.RunId); err == nil {
		x.runner.setSwarmSummary(renderSwarmSummary(reg))
	}
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: task.TaskID, Error: errString(syncErr)}
}

type messageInput struct {
	RunID       string            `json:"runId"`
	TaskID      string            `json:"taskId,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Title       string            `json:"title,omitempty"`
	Body        string            `json:"body,omitempty"`
	Attachments []string          `json:"attachments,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (x *orchestratorExecutor) handleMessage(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x.runner == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "runner missing"}
	}
	var in messageInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "runId is required"}
	}
	now := time.Now()
	msg := types.Message{
		MessageID:   "msg-" + uuid.NewString(),
		FromRunID:   x.runner.run.RunId,
		ToRunID:     runID,
		TaskID:      strings.TrimSpace(in.TaskID),
		Kind:        strings.TrimSpace(in.Kind),
		Title:       strings.TrimSpace(in.Title),
		Body:        strings.TrimSpace(in.Body),
		Attachments: in.Attachments,
		CreatedAt:   &now,
		Metadata:    in.Metadata,
	}
	if _, err := orchestrator.EnqueueMessage(x.runner.cfg, runID, msg); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	syncErr := orchestrator.SyncRegistry(x.runner.cfg, x.runner.run.RunId)
	if reg, err := orchestrator.LoadRegistryFile(x.runner.cfg, x.runner.run.RunId); err == nil {
		x.runner.setSwarmSummary(renderSwarmSummary(reg))
	}
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: msg.MessageID, Error: errString(syncErr)}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (x *orchestratorExecutor) handleSync(ctx context.Context) types.HostOpResponse {
	if x.runner == nil {
		return types.HostOpResponse{Op: types.HostOpOrchestratorSync, Ok: false, Error: "runner missing"}
	}
	if err := orchestrator.SyncRegistry(x.runner.cfg, x.runner.run.RunId); err != nil {
		return types.HostOpResponse{Op: types.HostOpOrchestratorSync, Ok: false, Error: err.Error()}
	}
	// Provide a compact summary of latest messages.
	reg, err := orchestrator.LoadRegistryFile(x.runner.cfg, x.runner.run.RunId)
	if err != nil {
		return types.HostOpResponse{Op: types.HostOpOrchestratorSync, Ok: true, Text: "synced"}
	}
	lines := []string{}
	for _, a := range reg.Agents {
		if a.LastMessage == nil {
			continue
		}
		msg := a.LastMessage
		body := strings.TrimSpace(msg.Body)
		title := strings.TrimSpace(msg.Title)
		s := a.RunID + ": "
		if title != "" {
			s += title
		} else if body != "" {
			s += body
		} else {
			continue
		}
		lines = append(lines, s)
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return types.HostOpResponse{Op: types.HostOpOrchestratorSync, Ok: true, Text: "synced"}
	}
	return types.HostOpResponse{Op: types.HostOpOrchestratorSync, Ok: true, Text: strings.Join(lines, "\n")}
}

func (x *orchestratorExecutor) handleList(ctx context.Context) types.HostOpResponse {
	if x.runner == nil {
		return types.HostOpResponse{Op: types.HostOpOrchestratorList, Ok: false, Error: "runner missing"}
	}
	children, err := orchestrator.ListChildRuns(x.runner.cfg, x.runner.run.SessionID, x.runner.run.RunId)
	if err != nil {
		return types.HostOpResponse{Op: types.HostOpOrchestratorList, Ok: false, Error: err.Error()}
	}
	out := make([]string, 0, len(children))
	for _, r := range children {
		out = append(out, r.RunId)
	}
	return types.HostOpResponse{Op: types.HostOpOrchestratorList, Ok: true, Entries: out}
}
