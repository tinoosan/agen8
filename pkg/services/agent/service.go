package agent

import (
	"context"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

// ServiceError is returned by the agent service for protocol-style errors.
// The RPC layer can translate it to protocol.ProtocolError using Code and Message.
type ServiceError struct {
	Code    int
	Message string
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Protocol-style error codes (same values as protocol package for easy mapping).
const (
	CodeInvalidParams  = -32602
	CodeThreadNotFound = -32002
	CodeItemNotFound   = -32004
	CodeInvalidState   = -32006
)

// AgentInfo is the domain representation of a listed agent (run).
type AgentInfo struct {
	RunID       string
	SessionID   string
	Status      string
	Goal        string
	ParentRunID string
	SpawnIndex  int
	Profile     string
	Role        string
	TeamID      string
	StartedAt   string // RFC3339Nano when set
	FinishedAt  string
}

// StartOptions is the input for starting a new agent run.
type StartOptions struct {
	SessionID          string
	Goal               string
	Profile            string
	Model              string
	MaxBytesForContext int
}

// StartResult is the output of Start.
type StartResult struct {
	RunID     string
	SessionID string
	Profile   string
	Model     string
}

// SessionProvider provides session and run persistence (subset of session.Service).
type SessionProvider interface {
	LoadSession(ctx context.Context, sessionID string) (types.Session, error)
	SaveSession(ctx context.Context, s types.Session) error
	LoadRun(ctx context.Context, runID string) (types.Run, error)
	SaveRun(ctx context.Context, run types.Run) error
}

// TaskLister lists tasks by filter (e.g. for role/team inference).
type TaskLister interface {
	ListTasks(ctx context.Context, filter state.TaskFilter) ([]types.Task, error)
}

// ActiveTaskCanceler cancels active tasks for a run (e.g. on pause).
type ActiveTaskCanceler interface {
	CancelActiveTasksByRun(ctx context.Context, runID, reason string) (int, error)
}

// RuntimeController controls run lifecycle (pause/resume/stop). Set via setter to break circular dependency.
type RuntimeController interface {
	PauseRun(ctx context.Context, runID string) error
	ResumeRun(ctx context.Context, runID string) error
	StopRun(ctx context.Context, runID string) error
}

// ServiceForRPC is the agent interface exposed to the RPC layer.
type ServiceForRPC interface {
	List(ctx context.Context, sessionID string) ([]AgentInfo, error)
	Start(ctx context.Context, opts StartOptions) (StartResult, error)
	Pause(ctx context.Context, runID, sessionID string) error
	Resume(ctx context.Context, runID, sessionID string) error
	InferRunRoleAndTeam(ctx context.Context, runID string) (role, teamID string)
}
