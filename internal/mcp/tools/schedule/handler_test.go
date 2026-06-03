package schedule

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	scheduledomain "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
)

type stubScheduleService struct {
	createReq scheduleapp.CreateParams
	cancelID  scheduledomain.EntryID
}

func (s *stubScheduleService) Create(_ context.Context, req scheduleapp.CreateParams) (scheduledomain.Entry, error) {
	s.createReq = req
	return scheduledomain.Entry{ID: "schedule-1", SpaceID: req.SpaceID, CreatedBy: req.CreatedBy, Status: scheduledomain.EntryStatusActive, Title: req.Title, Timing: req.Timing, Target: req.Target, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (s *stubScheduleService) Get(_ context.Context, id scheduledomain.EntryID) (scheduledomain.Entry, []scheduledomain.Run, error) {
	return scheduledomain.Entry{ID: id, Status: scheduledomain.EntryStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil, nil
}

func (s *stubScheduleService) List(_ context.Context, filter scheduledomain.Filter) ([]scheduledomain.Entry, error) {
	return []scheduledomain.Entry{{ID: "schedule-1", SpaceID: filter.SpaceID, Status: scheduledomain.EntryStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}}, nil
}

func (s *stubScheduleService) Update(_ context.Context, req scheduleapp.UpdateParams) (scheduledomain.Entry, error) {
	return scheduledomain.Entry{ID: req.EntryID, Status: scheduledomain.EntryStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (s *stubScheduleService) Cancel(_ context.Context, id scheduledomain.EntryID) (scheduledomain.Entry, error) {
	s.cancelID = id
	return scheduledomain.Entry{ID: id, Status: scheduledomain.EntryStatusCancelled, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func TestHandlerCreateRequiresOnlyRelevantFields(t *testing.T) {
	svc := &stubScheduleService{}
	runAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)
	result, err := NewHandler().Handle(context.Background(), CallContext{
		Schedules:     svc,
		SpaceID:       "space-1",
		ActorMemberID: "member-1",
	}, json.RawMessage(`{"action":"create","title":"Admission monitor","mode":"once","run_at":"`+runAt+`","target_kind":"task.create","target_member_id":"worker-1","task_title":"Check status","task_description":"Look for an update"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if svc.createReq.SpaceID != "space-1" || svc.createReq.CreatedBy != "member-1" {
		t.Fatalf("createReq identity=%+v", svc.createReq)
	}
	if svc.createReq.Target.TaskCreate.TargetMemberID != "worker-1" {
		t.Fatalf("target=%+v", svc.createReq.Target)
	}
	if result.Structured == nil {
		t.Fatal("Structured is nil")
	}
}

func TestHandlerRejectsNullFields(t *testing.T) {
	_, err := NewHandler().Handle(context.Background(), CallContext{Schedules: &stubScheduleService{}}, json.RawMessage(`{"action":"list","status":null}`))
	if err == nil {
		t.Fatal("Handle should reject null fields")
	}
}

func TestHandlerCancel(t *testing.T) {
	svc := &stubScheduleService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{Schedules: svc}, json.RawMessage(`{"action":"cancel","schedule_id":"schedule-1"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if svc.cancelID != "schedule-1" {
		t.Fatalf("cancelID=%q want schedule-1", svc.cancelID)
	}
}
