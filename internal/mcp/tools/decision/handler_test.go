package decision

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type recordingDecisionService struct {
	caller caller.Caller
	req    decisionapp.LogRequest
}

func (s *recordingDecisionService) Log(ctx context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	actor, err := caller.ContextResolver{}.ResolveCaller(ctx)
	if err != nil {
		return decisionapp.Result{}, err
	}
	s.caller = actor
	s.req = req
	return decisionapp.Result{
		ID:         "dec-1",
		Kind:       "log",
		Title:      req.Title,
		TaskRef:    req.TaskRef,
		MemberID:   req.MemberID,
		SourceType: "agent",
	}, nil
}

func TestHandleLogAddsSessionCallerContext(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "member-1",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"log",
		"title":"Choose API labels",
		"rationale":"Users need readable labels.",
		"task_ref":"task-1"
	}`))
	if err != nil {
		t.Fatalf("Handle log: %v", err)
	}
	if service.caller.UserID != "user-1" {
		t.Fatalf("caller user=%q want user-1", service.caller.UserID)
	}
	if service.caller.MemberID != member.ID("member-1") {
		t.Fatalf("caller member=%q want member-1", service.caller.MemberID)
	}
	if string(service.caller.ProjectID) != "project-1" {
		t.Fatalf("caller project=%q want project-1", service.caller.ProjectID)
	}
}

func TestHandleLogAcceptsContextField(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "member-1",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"log",
		"title":"Choose API labels",
		"rationale":"Users need readable labels.",
		"context":"Graph audit found raw member ids in task nodes.",
		"task_ref":"task-1"
	}`))
	if err != nil {
		t.Fatalf("Handle log: %v", err)
	}
	if service.req.Context != "Graph audit found raw member ids in task nodes." {
		t.Fatalf("context=%q want graph audit context", service.req.Context)
	}
}

// TestHandleLogRejectsMemberlessCaller locks the loud-failure half of the
// pre-registration affordance: the daemon hands actor-gated tools a member-LESS
// session for any caller that has not registered yet (resolveMCPSession returns no
// member, no error, when an in-band session ref matches nobody). decision.log must
// be the place that fails loudly, before the decision service is ever touched.
func TestHandleLogRejectsMemberlessCaller(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"log",
		"title":"Choose API labels",
		"rationale":"Users need readable labels."
	}`))
	if err == nil || !strings.Contains(err.Error(), "decision: member_id is required") {
		t.Fatalf("err=%v want decision member_id required", err)
	}
	if service.req.Title != "" {
		t.Fatalf("decision service ran despite member-less caller: %+v", service.req)
	}
}
