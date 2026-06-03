package app

import (
	"context"
	"testing"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

func TestTaskCreateExecutorCreatesScheduledTask(t *testing.T) {
	taskSvc := &spyTaskCreator{taskID: "task-1"}
	executor, err := NewTaskCreateExecutor(taskSvc)
	if err != nil {
		t.Fatalf("NewTaskCreateExecutor: %v", err)
	}
	entry := taskExecutorEntry(t)
	run, err := schedule.NewStartedRun(entry, *entry.NextRunAt, time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewStartedRun: %v", err)
	}
	result, err := executor.Execute(context.Background(), entry, run)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.TargetObjectID != "task-1" {
		t.Fatalf("TargetObjectID=%q want task-1", result.TargetObjectID)
	}
	if taskSvc.params.TaskKind != taskdomain.TaskKindScheduled {
		t.Fatalf("TaskKind=%q want %q", taskSvc.params.TaskKind, taskdomain.TaskKindScheduled)
	}
	if taskSvc.params.Metadata["scheduleEntryId"] != string(entry.ID) {
		t.Fatalf("metadata=%+v missing scheduleEntryId", taskSvc.params.Metadata)
	}
}

type spyTaskCreator struct {
	taskID taskdomain.TaskID
	params taskapp.CreateTaskParams
}

func (s *spyTaskCreator) Create(_ context.Context, params taskapp.CreateTaskParams) (taskdomain.Task, error) {
	s.params = params
	return taskdomain.Task{ID: s.taskID}, nil
}

func taskExecutorEntry(t *testing.T) schedule.Entry {
	t.Helper()
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	runAt := now.Add(-time.Minute)
	entry, err := schedule.NewEntry(schedule.NewEntryInput{
		ID:        "schedule-1",
		SpaceID:   spacedomain.SpaceID("space-1"),
		CreatedBy: schedule.ActorRef("member-1"),
		Title:     "Check admission status",
		Timing:    schedule.TimingExpression{Mode: schedule.TimingModeOnce, RunAt: &runAt},
		Target: schedule.Target{
			Kind: schedule.TargetKindTaskCreate,
			TaskCreate: schedule.TaskCreatePayload{
				TargetMemberID: member.ID("worker-1"),
				Title:          "Check admission status",
				Description:    "Look for a status update",
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return entry
}
