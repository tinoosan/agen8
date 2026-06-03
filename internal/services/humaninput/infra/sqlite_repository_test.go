package infra

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
)

func TestSQLiteRepositoryCreatePendingIsIdempotent(t *testing.T) {
	repo := newTestRepo(t)
	req := testRequest("hi-1")
	created, err := repo.CreatePending(context.Background(), req)
	if err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	duplicate := testRequest("hi-2")
	duplicate.ToolCallID = created.ToolCallID
	duplicate.IdempotencyKey = created.IdempotencyKey
	got, err := repo.CreatePending(context.Background(), duplicate)
	if err != nil {
		t.Fatalf("CreatePending duplicate: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("duplicate ID=%s want %s", got.ID, created.ID)
	}
}

func TestSQLiteRepositoryResolveRequiresPendingExpectedVersion(t *testing.T) {
	repo := newTestRepo(t)
	req, err := repo.CreatePending(context.Background(), testRequest("hi-1"))
	if err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	resolved, err := repo.Resolve(context.Background(), domain.ResolveMutation{
		ID:              req.ID,
		ExpectedVersion: req.Version,
		Status:          domain.StatusAnswered,
		Result:          json.RawMessage(`{"answers":[{"questionId":"q1","selectedOption":"A"}]}`),
		ResolvedAt:      req.CreatedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Status != domain.StatusAnswered || resolved.Version != 2 {
		t.Fatalf("unexpected resolved request: %+v", resolved)
	}
	if _, err := repo.Resolve(context.Background(), domain.ResolveMutation{
		ID:              req.ID,
		ExpectedVersion: req.Version,
		Status:          domain.StatusCancelled,
		Result:          json.RawMessage(`{"cancelled":true}`),
		ResolvedAt:      req.CreatedAt.Add(2 * time.Minute),
	}); err == nil {
		t.Fatal("second Resolve succeeded")
	}
}

func newTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func testRequest(id string) domain.Request {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	req, err := domain.NewPending(domain.PendingInput{
		ID:             domain.RequestID(id),
		ToolCallID:     "call-1",
		ToolName:       "decision",
		IdempotencyKey: "idem-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		AskerMemberID:  "member-1",
		ChannelID:      "channel-1",
		Declaration: domain.Declaration{
			Kind:    domain.PrimitiveQuestions,
			Payload: json.RawMessage(`{"questions":[{"id":"q1","text":"Choose","type":"multiple_choice","options":["A"]}]}`),
		},
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		panic(err)
	}
	return req
}
