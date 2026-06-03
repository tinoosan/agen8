package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

var humanInputTestTime = time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return humanInputTestTime }

type memoryRepo struct {
	records map[domain.RequestID]domain.Request
}

type recordingNotifier struct {
	requests []domain.Request
}

func (n *recordingNotifier) NotifyHumanInputChanged(_ context.Context, req domain.Request) error {
	n.requests = append(n.requests, req)
	return nil
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{records: map[domain.RequestID]domain.Request{}}
}

func (r *memoryRepo) CreatePending(_ context.Context, req domain.Request) (domain.Request, error) {
	r.records[req.ID] = req
	return req, nil
}

func (r *memoryRepo) Get(_ context.Context, id domain.RequestID) (domain.Request, error) {
	return r.records[id], nil
}

func (r *memoryRepo) FindByIdempotency(_ context.Context, projectID, toolName string, toolCallID domain.ToolCallID, idempotencyKey string) (domain.Request, error) {
	for _, req := range r.records {
		if req.ProjectID == projectID && req.ToolName == toolName && req.ToolCallID == toolCallID && req.IdempotencyKey == idempotencyKey {
			return req, nil
		}
	}
	return domain.Request{}, nil
}

func (r *memoryRepo) ListPending(_ context.Context, _ domain.Filter) ([]domain.Request, error) {
	out := make([]domain.Request, 0, len(r.records))
	for _, req := range r.records {
		if req.Status == domain.StatusPending {
			out = append(out, req)
		}
	}
	return out, nil
}

func (r *memoryRepo) Resolve(_ context.Context, mutation domain.ResolveMutation) (domain.Request, error) {
	req := r.records[mutation.ID]
	req.Status = mutation.Status
	req.Result = append(json.RawMessage(nil), mutation.Result...)
	req.ResolvedAt = &mutation.ResolvedAt
	req.Version++
	r.records[mutation.ID] = req
	return req, nil
}

func (r *memoryRepo) ExpireDue(context.Context, time.Time, int) (domain.ExpireBatch, error) {
	return domain.ExpireBatch{}, nil
}

func (r *memoryRepo) AbortByToolCall(context.Context, domain.ToolCallID, string) (domain.Request, error) {
	return domain.Request{}, nil
}

func TestServiceDeclarePersistsPendingRequest(t *testing.T) {
	repo := newMemoryRepo()
	svc, err := NewService(repo, fixedClock{}, func() string { return "hi-1" }, domain.DefaultValidator{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	notifier := &recordingNotifier{}
	svc.SetNotifier(notifier)
	payload := json.RawMessage(`{"questions":[{"id":"q1","text":"Choose","type":"multiple_choice","options":["A"]}]}`)
	req, err := svc.Declare(context.Background(), DeclareCommand{
		ToolCallID:     "call-1",
		ToolName:       "decision",
		IdempotencyKey: "idem-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		AskerMemberID:  "member-1",
		ChannelID:      "channel-1",
		Declaration:    domain.Declaration{Kind: domain.PrimitiveQuestions, Payload: payload},
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	if req.ID != "hi-1" || req.Status != domain.StatusPending || req.Version != 1 {
		t.Fatalf("unexpected request: %+v", req)
	}
	if req.CreatedAt != humanInputTestTime {
		t.Fatalf("CreatedAt=%v want %v", req.CreatedAt, humanInputTestTime)
	}
	if len(notifier.requests) != 1 || notifier.requests[0].ID != req.ID {
		t.Fatalf("notifier requests=%+v want request %s", notifier.requests, req.ID)
	}
}

func TestServiceResolveValidatesResult(t *testing.T) {
	repo := newMemoryRepo()
	svc, err := NewService(repo, fixedClock{}, func() string { return "hi-1" }, domain.DefaultValidator{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	notifier := &recordingNotifier{}
	svc.SetNotifier(notifier)
	req, err := svc.Declare(context.Background(), DeclareCommand{
		ToolCallID:     "call-1",
		ToolName:       "decision",
		IdempotencyKey: "idem-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		AskerMemberID:  "member-1",
		ChannelID:      "channel-1",
		Declaration:    domain.Declaration{Kind: domain.PrimitiveQuestions, Payload: json.RawMessage(`{"questions":[{"id":"q1","text":"Choose","type":"multiple_choice","options":["A"]}]}`)},
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	if _, err := svc.Resolve(context.Background(), ResolveCommand{RequestID: req.ID, ExpectedVersion: req.Version, Result: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("Resolve accepted empty questions result")
	}
	resolved, err := svc.Resolve(context.Background(), ResolveCommand{
		RequestID:       req.ID,
		ExpectedVersion: req.Version,
		Result:          json.RawMessage(`{"answers":[{"questionId":"q1","selectedOption":"A"}]}`),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Status != domain.StatusAnswered {
		t.Fatalf("Status=%s want answered", resolved.Status)
	}
	if len(notifier.requests) != 2 || notifier.requests[1].Status != domain.StatusAnswered {
		t.Fatalf("notifier requests=%+v want answered notification", notifier.requests)
	}
}
