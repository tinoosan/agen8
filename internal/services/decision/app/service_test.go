package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
)

func TestLogPersistsContextThroughDomainPayload(t *testing.T) {
	repo := &recordingRepository{}
	service, err := NewService(
		repo,
		fixedClock{now: time.Date(2026, 6, 5, 21, 15, 0, 0, time.UTC)},
		noopLinks{},
		noopEvents{},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.Log(context.Background(), LogRequest{
		ProjectID: "project-1",
		MemberID:  "member-1",
		Title:     "Choose API labels",
		Rationale: "Users need readable labels.",
		Context:   "Graph audit found raw member ids in task nodes.",
		TaskRef:   "task-1",
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if repo.created.Log == nil {
		t.Fatal("created decision log payload is nil")
	}
	if repo.created.Log.Context != "Graph audit found raw member ids in task nodes." {
		t.Fatalf("context=%q want graph audit context", repo.created.Log.Context)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type recordingRepository struct {
	created domain.Decision
}

func (r *recordingRepository) GetDecision(context.Context, domain.DecisionID) (domain.Decision, error) {
	return domain.Decision{}, nil
}

func (r *recordingRepository) ListDecisions(context.Context, domain.DecisionFilter) ([]domain.Decision, error) {
	return nil, nil
}

func (r *recordingRepository) ListDecisionsByKeyResult(context.Context, string) ([]domain.Decision, error) {
	return nil, nil
}

func (r *recordingRepository) CountDecisions(context.Context, domain.DecisionFilter) (int, error) {
	return 0, nil
}

func (r *recordingRepository) StatsDecisions(context.Context, domain.DecisionFilter) (domain.DecisionStats, error) {
	return domain.DecisionStats{}, nil
}

func (r *recordingRepository) ExportDecisions(context.Context, domain.DecisionFilter) ([]domain.Decision, error) {
	return nil, nil
}

func (r *recordingRepository) DecisionExistsByFingerprint(context.Context, string, string, string, string, time.Time) (bool, error) {
	return false, nil
}

func (r *recordingRepository) CreateDecision(_ context.Context, decision domain.Decision) error {
	r.created = decision
	return nil
}

func (r *recordingRepository) DeleteDecision(context.Context, domain.DecisionID) error {
	return nil
}

type noopLinks struct{}

func (noopLinks) DeleteLinksForNode(context.Context, string, string) error {
	return nil
}

type noopEvents struct{}

func (noopEvents) Publish(string, any) error {
	return nil
}
