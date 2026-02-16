package task

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/agent/hosttools"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// RunLoader loads a run by ID. Used by the Manager for CreateRetryTask.
type RunLoader interface {
	LoadRun(ctx context.Context, runID string) (types.Run, error)
}

// RetryEscalationCreator creates retry and escalation tasks (callback lifecycle).
// Implemented by Manager; consumed by the runtime supervisor (standalone daemon) and
// by the team runtime supervisor (team daemon). Escalation is a team feature: in
// team mode escalation tasks are assigned to the coordinator role.
type RetryEscalationCreator interface {
	CreateRetryTask(ctx context.Context, childRunID, feedback string) error
	CreateEscalationTask(ctx context.Context, callbackTaskID string, data hosttools.EscalationData) error
}

// ActiveTaskCanceler cancels all active tasks for a run (e.g. on pause/stop).
// Implemented by Manager; consumed by RPC, supervisor, team daemon, task_lifecycle.
type ActiveTaskCanceler interface {
	CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error)
}

// ArtifactIndexerProvider returns the artifact indexer when the underlying store supports it.
// Consumed by RPC artifact handlers.
type ArtifactIndexerProvider interface {
	ArtifactIndexer() (state.ArtifactIndexer, bool)
}

// TaskServiceForRPC is the task dependency for the RPC server: full store + cancel + artifact indexer.
type TaskServiceForRPC interface {
	state.TaskStore
	ActiveTaskCanceler
	ArtifactIndexerProvider
}

// TaskServiceForSupervisor is the task dependency for the runtime supervisor: store + retry/escalation + cancel.
type TaskServiceForSupervisor interface {
	state.TaskStore
	RetryEscalationCreator
	ActiveTaskCanceler
}

// TaskServiceForTeam is the task dependency for the team daemon: store + cancel.
type TaskServiceForTeam interface {
	state.TaskStore
	ActiveTaskCanceler
}
