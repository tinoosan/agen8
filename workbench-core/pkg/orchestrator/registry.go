package orchestrator

import (
	"encoding/json"
	"fmt"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

const (
	RegistryPath = "/agents/registry.json"
	MetricsPath  = "/agents/metrics.json"
)

type Registry struct {
	Version           string                `json:"version"`
	OrchestratorRunID string                `json:"orchestratorRunId"`
	CreatedAt         string                `json:"createdAt,omitempty"`
	Tasks             map[string]types.Task `json:"tasks,omitempty"`
	Agents            map[string]AgentState `json:"agents,omitempty"`
}

type AgentState struct {
	RunID         string     `json:"runId"`
	Status        string     `json:"status,omitempty"`
	CurrentTaskID string     `json:"currentTaskId,omitempty"`
	CurrentGoal   string     `json:"currentGoal,omitempty"`
	Plan          []string   `json:"plan,omitempty"`
	SpawnedAt     string     `json:"spawnedAt,omitempty"`
	LastPing      string     `json:"lastPing,omitempty"`
	Stats         AgentStats `json:"stats,omitempty"`
}

type AgentStats struct {
	TasksCompleted int     `json:"tasksCompleted,omitempty"`
	TasksFailed    int     `json:"tasksFailed,omitempty"`
	TokensIn       int     `json:"tokensIn,omitempty"`
	TokensOut      int     `json:"tokensOut,omitempty"`
	CostUSD        float64 `json:"costUsd,omitempty"`
}

type Metrics struct {
	Version string      `json:"version"`
	Tokens  TokenTotals `json:"tokens,omitempty"`
	CostUSD CostTotals  `json:"costUsd,omitempty"`
}

type TokenTotals struct {
	In  int `json:"in,omitempty"`
	Out int `json:"out,omitempty"`
}

type CostTotals struct {
	Total float64            `json:"total,omitempty"`
	ByRun map[string]float64 `json:"byRun,omitempty"`
}

func LoadRegistry(fs *vfs.FS) (Registry, error) {
	if fs == nil {
		return Registry{}, fmt.Errorf("fs is nil")
	}
	b, err := fs.Read(RegistryPath)
	if err != nil {
		return Registry{}, err
	}
	var reg Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		return Registry{}, fmt.Errorf("parse registry: %w", err)
	}
	return reg, nil
}

func SaveRegistry(fs *vfs.FS, reg Registry) error {
	if fs == nil {
		return fmt.Errorf("fs is nil")
	}
	b, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return fs.Write(RegistryPath, b)
}

func LoadMetrics(fs *vfs.FS) (Metrics, error) {
	if fs == nil {
		return Metrics{}, fmt.Errorf("fs is nil")
	}
	b, err := fs.Read(MetricsPath)
	if err != nil {
		return Metrics{}, err
	}
	var m Metrics
	if err := json.Unmarshal(b, &m); err != nil {
		return Metrics{}, fmt.Errorf("parse metrics: %w", err)
	}
	return m, nil
}

func SaveMetrics(fs *vfs.FS, metrics Metrics) error {
	if fs == nil {
		return fmt.Errorf("fs is nil")
	}
	b, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	return fs.Write(MetricsPath, b)
}
