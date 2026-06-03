package rpc

import (
	"context"
	"testing"
	"time"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

// noopDecisionDeps satisfies decisionapp.GraphLinkWriter,
// GraphLinkDeleter, and EventPublisher with no-ops for tests that
// don't care about decision side effects.
type noopDecisionDeps struct{}

func (noopDecisionDeps) Link(context.Context, graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	return graphdomain.GraphEdge{}, nil, nil
}
func (noopDecisionDeps) DeleteLinksForNode(context.Context, string, string) error { return nil }
func (noopDecisionDeps) Publish(string, any) error                                { return nil }

type stubDecisionRepo struct {
	decisions []domain.Decision
}

type stubUserDisplay struct {
	name string
}

func (s stubUserDisplay) CurrentUserDisplayName(context.Context) (string, error) {
	return s.name, nil
}

func (s *stubDecisionRepo) CreateDecision(_ context.Context, decision domain.Decision) error {
	s.decisions = append(s.decisions, decision)
	return nil
}

func (s *stubDecisionRepo) GetDecision(_ context.Context, id domain.DecisionID) (domain.Decision, error) {
	for _, decision := range s.decisions {
		if decision.ID == id {
			return decision, nil
		}
	}
	return domain.Decision{}, nil
}

func (s *stubDecisionRepo) DeleteDecision(_ context.Context, id domain.DecisionID) error {
	for i, decision := range s.decisions {
		if decision.ID == id {
			s.decisions = append(s.decisions[:i], s.decisions[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *stubDecisionRepo) ListDecisions(_ context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	out := make([]domain.Decision, 0, len(s.decisions))
	for _, decision := range s.decisions {
		if decision.ProjectID == filter.ProjectID {
			out = append(out, decision)
		}
	}
	return out, nil
}

func (s *stubDecisionRepo) ListDecisionsByKeyResult(_ context.Context, keyResultRef string) ([]domain.Decision, error) {
	return nil, nil
}

func (s *stubDecisionRepo) CountDecisions(_ context.Context, filter domain.DecisionFilter) (int, error) {
	decisions, err := s.ListDecisions(context.Background(), filter)
	if err != nil {
		return 0, err
	}
	return len(decisions), nil
}

func (s *stubDecisionRepo) ExportDecisions(_ context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	return s.ListDecisions(context.Background(), filter)
}

func (s *stubDecisionRepo) DecisionExistsByFingerprint(_ context.Context, projectID, sourceIdentity, title, taskRef string, since time.Time) (bool, error) {
	return false, nil
}

func TestList_ReturnsDecisionSpaceID(t *testing.T) {
	repo := &stubDecisionRepo{
		decisions: []domain.Decision{{
			ID:         "dec-1",
			ProjectID:  "proj-1",
			SpaceID:    "space-growth-ops",
			Source:     domain.DecisionSourceAgent,
			Title:      "Rebalance roadmap",
			Confidence: 0.88,
			CreatedAt:  time.Now().UTC(),
			Log:        &domain.LogPayload{Rationale: "New input from operator"},
		}},
	}
	stub := noopDecisionDeps{}
	svc, err := decisionapp.NewService(repo, domain.SystemClock{}, stub, stub, stub, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(svc)

	result, err := handler.List(context.Background(), protocol.DecisionListParams{
		ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Decisions) != 1 {
		t.Fatalf("len(result.Decisions)=%d want 1", len(result.Decisions))
	}
	if got := result.Decisions[0].SpaceID; got != "space-growth-ops" {
		t.Fatalf("decision.spaceId=%q want space-growth-ops", got)
	}
}

func TestList_ResolvesOperatorDecisionToCurrentUserName(t *testing.T) {
	repo := &stubDecisionRepo{
		decisions: []domain.Decision{{
			ID:             "dec-operator",
			ProjectID:      "proj-1",
			Source:         domain.DecisionSourceOperator,
			SourceIdentity: "operator",
			Title:          "Resolved escalation",
			Confidence:     0.8,
			CreatedAt:      time.Now().UTC(),
			Log:            &domain.LogPayload{Rationale: "Resolved by the signed-in user"},
		}},
	}
	stub := noopDecisionDeps{}
	svc, err := decisionapp.NewService(repo, domain.SystemClock{}, stub, stub, stub, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(svc)
	handler.SetUserDisplayLookup(stubUserDisplay{name: "Santino Onyeme"})

	result, err := handler.List(context.Background(), protocol.DecisionListParams{
		ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Decisions) != 1 {
		t.Fatalf("len(result.Decisions)=%d want 1", len(result.Decisions))
	}
	if got := result.Decisions[0].MemberName; got != "Santino Onyeme" {
		t.Fatalf("decision.memberName=%q want Santino Onyeme", got)
	}
	if got := result.Decisions[0].MemberID; got != "operator" {
		t.Fatalf("decision.memberId=%q want operator", got)
	}
}
