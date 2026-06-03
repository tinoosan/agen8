package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	schedulerpc "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/rpc"
)

var rpcScheduleTestNow = time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

func TestRegisterScheduleDispatchCreateAndList(t *testing.T) {
	svc := newRPCScheduleService(t)
	reg := NewRegistry()
	if err := RegisterSchedule(reg, svc); err != nil {
		t.Fatalf("RegisterSchedule returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "schedule.create",
		"params": {
			"spaceId": "space-1",
			"title": "Check admission status",
			"mode": "once",
			"runAt": "2026-05-31T13:00:00Z",
			"targetMemberId": "member-worker",
			"taskDescription": "Look for status updates and summarize what changed."
		}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var created schedulerpc.ScheduleCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}
	if created.Schedule.EntryID == "" || created.Schedule.MemberID != "member-worker" {
		t.Fatalf("created schedule=%+v", created.Schedule)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "schedule.list",
		"params": { "spaceId": "space-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var listed schedulerpc.ScheduleListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if len(listed.Schedules) != 1 || listed.Schedules[0].EntryID != created.Schedule.EntryID {
		t.Fatalf("listed schedules=%+v want created schedule", listed.Schedules)
	}
}

func TestRegisterScheduleRequiresIdentity(t *testing.T) {
	svc := newRPCScheduleService(t)
	reg := NewRegistry()
	if err := RegisterSchedule(reg, svc); err != nil {
		t.Fatalf("RegisterSchedule returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "schedule.list",
		"params": { "spaceId": "space-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func TestRegisterScheduleMapsInvalidParams(t *testing.T) {
	svc := newRPCScheduleService(t)
	reg := NewRegistry()
	if err := RegisterSchedule(reg, svc); err != nil {
		t.Fatalf("RegisterSchedule returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "schedule.create",
		"params": {
			"spaceId": "space-1",
			"mode": "once",
			"runAt": "2026-05-31T13:00:00Z",
			"targetMemberId": "member-worker",
			"taskDescription": "Look for status updates and summarize what changed."
		}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func newRPCScheduleService(t *testing.T) *scheduleapp.Service {
	t.Helper()
	svc, err := scheduleapp.NewService(newRPCScheduleRepo(), schedule.FixedClock{T: rpcScheduleTestNow}, nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	return svc
}

type rpcScheduleRepo struct {
	entries map[schedule.EntryID]schedule.Entry
	runs    map[schedule.RunID]schedule.Run
}

func newRPCScheduleRepo() *rpcScheduleRepo {
	return &rpcScheduleRepo{
		entries: make(map[schedule.EntryID]schedule.Entry),
		runs:    make(map[schedule.RunID]schedule.Run),
	}
}

func (r *rpcScheduleRepo) Get(_ context.Context, id schedule.EntryID) (schedule.Entry, error) {
	entry, ok := r.entries[id]
	if !ok {
		return schedule.Entry{}, schedule.ErrNotFound
	}
	return entry, nil
}

func (r *rpcScheduleRepo) List(_ context.Context, filter schedule.Filter) ([]schedule.Entry, error) {
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

func (r *rpcScheduleRepo) ListDue(_ context.Context, now time.Time, limit int) ([]schedule.Entry, error) {
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

func (r *rpcScheduleRepo) ListRuns(_ context.Context, filter schedule.RunFilter) ([]schedule.Run, error) {
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

func (r *rpcScheduleRepo) Create(_ context.Context, entry schedule.Entry) error {
	r.entries[entry.ID] = entry
	return nil
}

func (r *rpcScheduleRepo) Update(_ context.Context, entry schedule.Entry) error {
	if _, ok := r.entries[entry.ID]; !ok {
		return schedule.ErrNotFound
	}
	r.entries[entry.ID] = entry
	return nil
}

func (r *rpcScheduleRepo) ClaimDue(_ context.Context, run schedule.Run) (schedule.Run, bool, error) {
	for _, existing := range r.runs {
		if existing.EntryID == run.EntryID && existing.DueAt.Equal(run.DueAt) {
			return existing, false, nil
		}
	}
	r.runs[run.ID] = run
	return run, true, nil
}

func (r *rpcScheduleRepo) UpdateRun(_ context.Context, run schedule.Run) error {
	if _, ok := r.runs[run.ID]; !ok {
		return schedule.ErrRunNotFound
	}
	r.runs[run.ID] = run
	return nil
}
