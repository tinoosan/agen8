package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

type rpcDecisionRepo struct {
	decisions []domain.Decision
}

func (r *rpcDecisionRepo) CreateDecision(_ context.Context, decision domain.Decision) error {
	r.decisions = append(r.decisions, decision)
	return nil
}

func (r *rpcDecisionRepo) GetDecision(_ context.Context, id domain.DecisionID) (domain.Decision, error) {
	for _, decision := range r.decisions {
		if decision.ID == id {
			return decision, nil
		}
	}
	return domain.Decision{}, fmt.Errorf("decision not found")
}

func (r *rpcDecisionRepo) DeleteDecision(_ context.Context, id domain.DecisionID) error {
	for i, decision := range r.decisions {
		if decision.ID == id {
			r.decisions = append(r.decisions[:i], r.decisions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("decision not found")
}

func (r *rpcDecisionRepo) ListDecisions(_ context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	out := make([]domain.Decision, 0, len(r.decisions))
	for _, decision := range r.decisions {
		if decision.ProjectID == filter.ProjectID {
			out = append(out, decision)
		}
	}
	return out, nil
}

func (r *rpcDecisionRepo) ListDecisionsByKeyResult(context.Context, string) ([]domain.Decision, error) {
	return nil, nil
}

func (r *rpcDecisionRepo) CountDecisions(ctx context.Context, filter domain.DecisionFilter) (int, error) {
	decisions, err := r.ListDecisions(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(decisions), nil
}

func (r *rpcDecisionRepo) ExportDecisions(ctx context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	return r.ListDecisions(ctx, filter)
}

func (r *rpcDecisionRepo) DecisionExistsByFingerprint(context.Context, string, string, string, string, time.Time) (bool, error) {
	return false, nil
}

type rpcDecisionDeps struct{}

func (rpcDecisionDeps) Link(context.Context, graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	return graphdomain.GraphEdge{}, nil, nil
}
func (rpcDecisionDeps) DeleteLinksForNode(context.Context, string, string) error { return nil }
func (rpcDecisionDeps) Publish(string, any) error                                { return nil }

func TestRegisterDecisionDispatchCreateAndList(t *testing.T) {
	svc := newRPCDecisionService(t)
	reg := NewRegistry()
	if err := RegisterDecision(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterDecision returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "decision.create",
		"params": {
			"projectId": "proj-1",
			"spaceId": "space-1",
			"source": "agent",
			"sourceIdentity": "member-coordinator",
			"title": "Use Redis for caching",
			"rationale": "Lower latency than database reads",
			"confidence": 0.85
		}
	}`))
	if err != nil {
		t.Fatalf("Handle create returned error: %v", err)
	}
	createResp := decodeRPCResponse(t, raw)
	if createResp.Error != nil {
		t.Fatalf("create response error=%+v", createResp.Error)
	}
	var createResult protocol.DecisionCreateResult
	if err := json.Unmarshal(createResp.Result, &createResult); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}
	if createResult.Decision.ID == "" || createResult.Decision.ProjectID != "proj-1" {
		t.Fatalf("create decision result=%+v", createResult.Decision)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "decision.list",
		"params": {"projectId": "proj-1"}
	}`))
	if err != nil {
		t.Fatalf("Handle list returned error: %v", err)
	}
	listResp := decodeRPCResponse(t, raw)
	if listResp.Error != nil {
		t.Fatalf("list response error=%+v", listResp.Error)
	}
	var listResult protocol.DecisionListResult
	if err := json.Unmarshal(listResp.Result, &listResult); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if len(listResult.Decisions) != 1 {
		t.Fatalf("len(decisions)=%d want 1", len(listResult.Decisions))
	}
	if listResult.Decisions[0].Title != "Use Redis for caching" {
		t.Fatalf("decision title=%q", listResult.Decisions[0].Title)
	}
}

func TestRegisterDecisionRequiresIdentity(t *testing.T) {
	reg := NewRegistry()
	if err := RegisterDecision(reg, newRPCDecisionService(t), nil, nil); err != nil {
		t.Fatalf("RegisterDecision returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "decision.list",
		"params": {"projectId": "proj-1"}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func newRPCDecisionService(t *testing.T) *decisionapp.Service {
	t.Helper()
	deps := rpcDecisionDeps{}
	svc, err := decisionapp.NewService(&rpcDecisionRepo{}, domain.SystemClock{}, deps, deps, deps, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("decision service: %v", err)
	}
	return svc
}
