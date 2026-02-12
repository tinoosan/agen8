package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
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
