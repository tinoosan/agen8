package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestTeamIsIdle_IgnoresHeartbeatTasks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(t.TempDir()))
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}
	now := time.Now().UTC()
	ctx := context.Background()

	if err := store.CreateTask(ctx, types.Task{
		TaskID:     "heartbeat-1",
		SessionID:  "sess-1",
		RunID:      "run-1",
		TeamID:     "team-1",
		TaskKind:   state.TaskKindHeartbeat,
		Status:     types.TaskStatusPending,
		Goal:       "heartbeat",
		CreatedAt:  &now,
		AssignedTo: "run-1",
	}); err != nil {
		t.Fatalf("create heartbeat task: %v", err)
	}

	if !teamIsIdle(ctx, store, "team-1") {
		t.Fatalf("expected team to be idle with only heartbeat tasks")
	}
}

func TestTeamIsIdle_BlocksOnNonHeartbeatTasks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(t.TempDir()))
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}
	now := time.Now().UTC()
	ctx := context.Background()

	if err := store.CreateTask(ctx, types.Task{
		TaskID:     "task-1",
		SessionID:  "sess-1",
		RunID:      "run-1",
		TeamID:     "team-1",
		TaskKind:   state.TaskKindCallback,
		Status:     types.TaskStatusPending,
		Goal:       "regular work",
		CreatedAt:  &now,
		AssignedTo: "run-1",
	}); err != nil {
		t.Fatalf("create non-heartbeat task: %v", err)
	}

	if teamIsIdle(ctx, store, "team-1") {
		t.Fatalf("expected team to be non-idle with regular pending tasks")
	}
}

func TestBuildTeamRPCServerConfig_AcceptsRoleSessionThread(t *testing.T) {
	sessionStore := &memorySessionStore{
		sessions: map[string]types.Session{
			"coord-sess":  {SessionID: "coord-sess"},
			"worker-sess": {SessionID: "worker-sess"},
		},
	}
	srvCfg := buildTeamRPCServerConfig(
		RPCServerConfig{},
		config.Config{},
		RunChatOptions{},
		types.Run{SessionID: "coord-sess", RunID: "coord-run"},
		nil,
		sessionStore,
		[]teamRoleRuntime{
			{run: types.Run{SessionID: "worker-sess", RunID: "worker-run"}},
		},
		nil,
		&sync.Mutex{},
		map[string]context.CancelFunc{},
	)

	err := srvCfg.AgentPause(context.Background(), "worker-sess", "")
	pErr, ok := err.(*protocol.ProtocolError)
	if !ok {
		t.Fatalf("expected protocol error, got %T", err)
	}
	if pErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("protocol code=%d want=%d", pErr.Code, protocol.CodeInvalidParams)
	}
}

func TestBuildTeamRPCServerConfig_RejectsUnknownThread(t *testing.T) {
	sessionStore := &memorySessionStore{
		sessions: map[string]types.Session{
			"coord-sess": {SessionID: "coord-sess"},
		},
	}
	srvCfg := buildTeamRPCServerConfig(
		RPCServerConfig{},
		config.Config{},
		RunChatOptions{},
		types.Run{SessionID: "coord-sess", RunID: "coord-run"},
		nil,
		sessionStore,
		nil,
		nil,
		&sync.Mutex{},
		map[string]context.CancelFunc{},
	)

	_, err := srvCfg.ControlSetModel(context.Background(), "missing-thread", "", "openai/gpt-5-mini")
	pErr, ok := err.(*protocol.ProtocolError)
	if !ok {
		t.Fatalf("expected protocol error, got %T", err)
	}
	if pErr.Code != protocol.CodeThreadNotFound {
		t.Fatalf("protocol code=%d want=%d", pErr.Code, protocol.CodeThreadNotFound)
	}
}
