package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// OrchestratorSpawnTool creates a child run and optionally enqueues the initial task.
type OrchestratorSpawnTool struct{}

type spawnArgs struct {
	Goal     string         `json:"goal"`
	Priority string         `json:"priority,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (t *OrchestratorSpawnTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "orchestrator_spawn",
			Description: "Spawn a worker run for a goal",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal":     map[string]any{"type": "string", "description": "goal for the new worker"},
					"priority": map[string]any{"type": "string", "description": "optional priority (P0-P3)"},
					"metadata": map[string]any{"type": "object", "description": "optional task metadata"},
				},
				"required":             []any{"goal"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *OrchestratorSpawnTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var in spawnArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return types.HostOpRequest{}, err
	}
	if strings.TrimSpace(in.Goal) == "" {
		return types.HostOpRequest{}, fmt.Errorf("goal is required")
	}
	return types.HostOpRequest{Op: types.HostOpOrchestratorSpawn, Input: args}, nil
}

// OrchestratorTaskTool enqueues a task onto an existing worker.
type OrchestratorTaskTool struct{}

type taskArgs struct {
	RunID    string         `json:"runId"`
	Goal     string         `json:"goal"`
	WaitFor  []string       `json:"waitFor,omitempty"`
	Priority string         `json:"priority,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (t *OrchestratorTaskTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "orchestrator_task",
			Description: "Enqueue a task for a specific worker run",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runId":    map[string]any{"type": "string"},
					"goal":     map[string]any{"type": "string"},
					"waitFor":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"priority": map[string]any{"type": "string"},
					"inputs":   map[string]any{"type": "object"},
					"metadata": map[string]any{"type": "object"},
				},
				"required":             []any{"runId", "goal"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *OrchestratorTaskTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var in taskArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return types.HostOpRequest{}, err
	}
	if strings.TrimSpace(in.RunID) == "" || strings.TrimSpace(in.Goal) == "" {
		return types.HostOpRequest{}, fmt.Errorf("runId and goal are required")
	}
	return types.HostOpRequest{Op: types.HostOpOrchestratorTask, Input: args}, nil
}

// OrchestratorMessageTool sends a message to a worker's inbox.
type OrchestratorMessageTool struct{}

type messageArgs struct {
	RunID       string            `json:"runId"`
	TaskID      string            `json:"taskId,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Title       string            `json:"title,omitempty"`
	Body        string            `json:"body,omitempty"`
	Attachments []string          `json:"attachments,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (t *OrchestratorMessageTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "orchestrator_message",
			Description: "Send a message to a worker run",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"runId":       map[string]any{"type": "string"},
					"taskId":      map[string]any{"type": "string"},
					"kind":        map[string]any{"type": "string"},
					"title":       map[string]any{"type": "string"},
					"body":        map[string]any{"type": "string"},
					"attachments": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"metadata":    map[string]any{"type": "object"},
				},
				"required":             []any{"runId"},
				"additionalProperties": false,
			},
		},
	}
}

func (t *OrchestratorMessageTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var in messageArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return types.HostOpRequest{}, err
	}
	if strings.TrimSpace(in.RunID) == "" {
		return types.HostOpRequest{}, fmt.Errorf("runId is required")
	}
	return types.HostOpRequest{Op: types.HostOpOrchestratorMessage, Input: args}, nil
}

// OrchestratorSyncTool refreshes registry/metrics and returns recent messages.
type OrchestratorSyncTool struct{}

func (t *OrchestratorSyncTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "orchestrator_sync",
			Description: "Refresh swarm registry/metrics and fetch recent worker messages",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

func (t *OrchestratorSyncTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	return types.HostOpRequest{Op: types.HostOpOrchestratorSync, Input: args}, nil
}

// OrchestratorListTool lists child run IDs.
type OrchestratorListTool struct{}

func (t *OrchestratorListTool) Definition() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "orchestrator_list",
			Description: "List worker runs spawned by this orchestrator",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

func (t *OrchestratorListTool) Execute(ctx context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	return types.HostOpRequest{Op: types.HostOpOrchestratorList, Input: args}, nil
}
