package rpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
)

var rpcTestNow = time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

func TestHandlerCreateListAndCancelSchedule(t *testing.T) {
	handler := newHandlerForTest(t)
	runAt := rpcTestNow.Add(time.Hour)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"})

	created, err := handler.Create(ctx, ScheduleCreateParams{
		SpaceID:         "space-1",
		Title:           "Check admission status",
		Mode:            "once",
		RunAt:           &runAt,
		TargetMemberID:  "member-worker",
		TaskDescription: "Look for status updates and summarize what changed.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Schedule.EntryID == "" {
		t.Fatalf("created schedule has empty id: %+v", created.Schedule)
	}
	if created.Schedule.SpaceID != "space-1" || created.Schedule.MemberID != "member-worker" {
		t.Fatalf("created schedule=%+v", created.Schedule)
	}
	if created.Schedule.ScheduleType != "one_off" || created.Schedule.ScheduleExpr != runAt.UTC().Format(time.RFC3339) {
		t.Fatalf("created timing=%+v", created.Schedule)
	}

	listed, err := handler.List(ctx, ScheduleListParams{SpaceID: "space-1"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed.Schedules) != 1 || listed.Schedules[0].EntryID != created.Schedule.EntryID {
		t.Fatalf("listed schedules=%+v want created schedule", listed.Schedules)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].EntryID != created.Schedule.EntryID {
		t.Fatalf("listed entries=%+v want compatibility entries", listed.Entries)
	}

	cancelled, err := handler.Cancel(ctx, ScheduleCancelParams{ScheduleID: created.Schedule.EntryID})
	if err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if cancelled.Schedule.Status != string(schedule.EntryStatusCancelled) {
		t.Fatalf("cancelled schedule status=%q want cancelled", cancelled.Schedule.Status)
	}
}

func TestHandlerListIncludesScheduleRuns(t *testing.T) {
	repo := newRPCMemoryRepo()
	svc, err := scheduleapp.NewService(repo, schedule.FixedClock{T: rpcTestNow}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	handler := NewHandler(svc)
	runAt := rpcTestNow.Add(-time.Hour)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"})

	created, err := handler.Create(ctx, ScheduleCreateParams{
		SpaceID:         "space-1",
		Title:           "Check admission status",
		Mode:            "once",
		RunAt:           &runAt,
		TargetMemberID:  "member-worker",
		TaskDescription: "Look for status updates and summarize what changed.",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	entry := repo.entries[schedule.EntryID(created.Schedule.EntryID)]
	run, err := schedule.NewStartedRun(entry, runAt, rpcTestNow)
	if err != nil {
		t.Fatalf("NewStartedRun returned error: %v", err)
	}
	finished, err := run.Succeed("task-123", rpcTestNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("Succeed returned error: %v", err)
	}
	repo.runs[finished.ID] = finished

	listed, err := handler.List(ctx, ScheduleListParams{SpaceID: "space-1"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed.Schedules) != 1 || len(listed.Schedules[0].Runs) != 1 {
		t.Fatalf("listed schedules=%+v want one run", listed.Schedules)
	}
	if listed.Schedules[0].Runs[0].TargetObjectID != "task-123" {
		t.Fatalf("run target=%q want task-123", listed.Schedules[0].Runs[0].TargetObjectID)
	}
}

func TestHandlerCreateRejectsMissingTitle(t *testing.T) {
	handler := newHandlerForTest(t)
	runAt := rpcTestNow.Add(time.Hour)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"})

	_, err := handler.Create(ctx, ScheduleCreateParams{
		SpaceID:         "space-1",
		Mode:            "once",
		RunAt:           &runAt,
		TargetMemberID:  "member-worker",
		TaskDescription: "Look for status updates and summarize what changed.",
	})
	if err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("Create error=%v want title validation error", err)
	}
	coded, ok := err.(interface{ RPCCode() int })
	if !ok || coded.RPCCode() != -32602 {
		t.Fatalf("Create error=%T %[1]v want invalid params RPC code", err)
	}
}

func newHandlerForTest(t *testing.T) *Handler {
	t.Helper()
	svc, err := scheduleapp.NewService(newRPCMemoryRepo(), schedule.FixedClock{T: rpcTestNow}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return NewHandler(svc)
}

type rpcMemoryRepo struct {
	entries map[schedule.EntryID]schedule.Entry
	runs    map[schedule.RunID]schedule.Run
}

func newRPCMemoryRepo() *rpcMemoryRepo {
	return &rpcMemoryRepo{
		entries: make(map[schedule.EntryID]schedule.Entry),
		runs:    make(map[schedule.RunID]schedule.Run),
	}
}

func (r *rpcMemoryRepo) Get(_ context.Context, id schedule.EntryID) (schedule.Entry, error) {
	entry, ok := r.entries[id]
	if !ok {
		return schedule.Entry{}, schedule.ErrNotFound
	}
	return entry, nil
}

func (r *rpcMemoryRepo) List(_ context.Context, filter schedule.Filter) ([]schedule.Entry, error) {
	out := make([]schedule.Entry, 0, len(r.entries))
	for _, entry := range r.entries {
		if filter.SpaceID != "" && entry.SpaceID != filter.SpaceID {
			continue
		}
		if filter.Status != "" && entry.Status != filter.Status {
			continue
		}
		out = append(out, entry)
		if filter.Limit > 0 && len(out) == filter.Limit {
			break
		}
	}
	return out, nil
}

func (r *rpcMemoryRepo) ListDue(_ context.Context, now time.Time, limit int) ([]schedule.Entry, error) {
	out := make([]schedule.Entry, 0, limit)
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

func (r *rpcMemoryRepo) ListRuns(_ context.Context, filter schedule.RunFilter) ([]schedule.Run, error) {
	out := make([]schedule.Run, 0, len(r.runs))
	for _, run := range r.runs {
		if filter.EntryID != "" && run.EntryID != filter.EntryID {
			continue
		}
		if filter.SpaceID != "" && run.SpaceID != filter.SpaceID {
			continue
		}
		out = append(out, run)
		if filter.Limit > 0 && len(out) == filter.Limit {
			break
		}
	}
	return out, nil
}

func (r *rpcMemoryRepo) Create(_ context.Context, entry schedule.Entry) error {
	r.entries[entry.ID] = entry
	return nil
}

func (r *rpcMemoryRepo) Update(_ context.Context, entry schedule.Entry) error {
	if _, ok := r.entries[entry.ID]; !ok {
		return schedule.ErrNotFound
	}
	r.entries[entry.ID] = entry
	return nil
}

func (r *rpcMemoryRepo) ClaimDue(_ context.Context, run schedule.Run) (schedule.Run, bool, error) {
	for _, existing := range r.runs {
		if existing.EntryID == run.EntryID && existing.DueAt.Equal(run.DueAt) {
			return existing, false, nil
		}
	}
	r.runs[run.ID] = run
	return run, true, nil
}

func (r *rpcMemoryRepo) UpdateRun(_ context.Context, run schedule.Run) error {
	if _, ok := r.runs[run.ID]; !ok {
		return schedule.ErrRunNotFound
	}
	r.runs[run.ID] = run
	return nil
}
