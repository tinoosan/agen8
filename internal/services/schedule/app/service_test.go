package app

import (
	"context"
	"errors"
	"testing"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

var appTestNow = time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

func TestServiceRunDueExecutesTargetAndAdvancesEntry(t *testing.T) {
	repo := newMemoryRepo()
	clock := schedule.FixedClock{T: appTestNow}
	svc, err := NewService(repo, clock, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	executor := &spyExecutor{targetObjectID: "task-1"}
	if err := svc.RegisterExecutor(schedule.TargetKindTaskCreate, executor); err != nil {
		t.Fatalf("RegisterExecutor: %v", err)
	}
	runAt := appTestNow.Add(-time.Minute)
	entry, err := svc.Create(context.Background(), CreateParams{
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
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	runs, err := svc.RunDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != schedule.RunStatusSucceeded || runs[0].TargetObjectID != "task-1" {
		t.Fatalf("runs=%+v want succeeded task run", runs)
	}
	if executor.entry.ID != entry.ID {
		t.Fatalf("executor entry=%s want %s", executor.entry.ID, entry.ID)
	}
	loaded, _, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Status != schedule.EntryStatusTriggered || loaded.NextRunAt != nil {
		t.Fatalf("loaded=%+v want triggered one-shot entry", loaded)
	}
}

func TestServiceRunDueRecordsFailureWhenExecutorFails(t *testing.T) {
	repo := newMemoryRepo()
	svc, err := NewService(repo, schedule.FixedClock{T: appTestNow}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.RegisterExecutor(schedule.TargetKindTaskCreate, &spyExecutor{err: errors.New("task service unavailable")}); err != nil {
		t.Fatalf("RegisterExecutor: %v", err)
	}
	runAt := appTestNow.Add(-time.Minute)
	entry, err := svc.Create(context.Background(), CreateParams{
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
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	runs, err := svc.RunDue(context.Background(), 10)
	if err == nil {
		t.Fatalf("RunDue should return executor error")
	}
	if len(runs) != 1 || runs[0].Status != schedule.RunStatusFailed {
		t.Fatalf("runs=%+v want failed run", runs)
	}
	loaded, history, err := svc.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Status != schedule.EntryStatusTriggered {
		t.Fatalf("Status=%q want triggered", loaded.Status)
	}
	if len(history) != 1 || history[0].Error != "task service unavailable" {
		t.Fatalf("history=%+v want recorded error", history)
	}
}

type spyExecutor struct {
	targetObjectID string
	err            error
	entry          schedule.Entry
	run            schedule.Run
}

func (e *spyExecutor) Execute(_ context.Context, entry schedule.Entry, run schedule.Run) (TargetResult, error) {
	e.entry = entry
	e.run = run
	if e.err != nil {
		return TargetResult{}, e.err
	}
	return TargetResult{TargetObjectID: e.targetObjectID}, nil
}

type memoryRepo struct {
	entries map[schedule.EntryID]schedule.Entry
	runs    map[schedule.RunID]schedule.Run
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		entries: make(map[schedule.EntryID]schedule.Entry),
		runs:    make(map[schedule.RunID]schedule.Run),
	}
}

func (r *memoryRepo) Get(_ context.Context, id schedule.EntryID) (schedule.Entry, error) {
	entry, ok := r.entries[id]
	if !ok {
		return schedule.Entry{}, schedule.ErrNotFound
	}
	return entry, nil
}

func (r *memoryRepo) List(_ context.Context, filter schedule.Filter) ([]schedule.Entry, error) {
	var out []schedule.Entry
	for _, entry := range r.entries {
		if filter.SpaceID != "" && entry.SpaceID != filter.SpaceID {
			continue
		}
		if filter.Status != "" && entry.Status != filter.Status {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (r *memoryRepo) ListDue(_ context.Context, now time.Time, limit int) ([]schedule.Entry, error) {
	var out []schedule.Entry
	for _, entry := range r.entries {
		if !entry.IsDue(now) {
			continue
		}
		out = append(out, entry)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r *memoryRepo) ListRuns(_ context.Context, filter schedule.RunFilter) ([]schedule.Run, error) {
	var out []schedule.Run
	for _, run := range r.runs {
		if filter.EntryID != "" && run.EntryID != filter.EntryID {
			continue
		}
		if filter.SpaceID != "" && run.SpaceID != filter.SpaceID {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *memoryRepo) Create(_ context.Context, entry schedule.Entry) error {
	r.entries[entry.ID] = entry
	return nil
}

func (r *memoryRepo) Update(_ context.Context, entry schedule.Entry) error {
	if _, ok := r.entries[entry.ID]; !ok {
		return schedule.ErrNotFound
	}
	r.entries[entry.ID] = entry
	return nil
}

func (r *memoryRepo) ClaimDue(_ context.Context, run schedule.Run) (schedule.Run, bool, error) {
	for _, existing := range r.runs {
		if existing.EntryID == run.EntryID && existing.DueAt.Equal(run.DueAt) {
			return existing, false, nil
		}
	}
	r.runs[run.ID] = run
	return run, true, nil
}

func (r *memoryRepo) UpdateRun(_ context.Context, run schedule.Run) error {
	if _, ok := r.runs[run.ID]; !ok {
		return schedule.ErrRunNotFound
	}
	r.runs[run.ID] = run
	return nil
}
