package decision

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type recordingDecisionService struct {
	caller caller.Caller
}

func (s *recordingDecisionService) Log(ctx context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	actor, err := caller.ContextResolver{}.ResolveCaller(ctx)
	if err != nil {
		return decisionapp.Result{}, err
	}
	s.caller = actor
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
