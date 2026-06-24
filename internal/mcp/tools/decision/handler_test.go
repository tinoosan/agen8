package decision

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
)

type recordingDecisionService struct {
	caller    caller.Caller
	req       decisionapp.LogRequest
	deleteID  decisiondomain.DecisionID
	deleteErr error
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

func (s *recordingDecisionService) Delete(ctx context.Context, id decisiondomain.DecisionID) error {
	actor, err := caller.ContextResolver{}.ResolveCaller(ctx)
	if err != nil {
		return err
	}
	s.caller = actor
	s.deleteID = id
	return s.deleteErr
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
	if service.caller.MemberID != "member-1" {
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

func TestHandleDeleteAddsSessionCallerContext(t *testing.T) {
	service := &recordingDecisionService{}
	result, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "member-1",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"delete",
		"decision_id":"dec-1"
	}`))
	if err != nil {
		t.Fatalf("Handle delete: %v", err)
	}
	if service.deleteID != "dec-1" {
		t.Fatalf("delete id=%q want dec-1", service.deleteID)
	}
	if service.caller.UserID != "user-1" {
		t.Fatalf("caller user=%q want user-1", service.caller.UserID)
	}
	if service.caller.MemberID != "member-1" {
		t.Fatalf("caller member=%q want member-1", service.caller.MemberID)
	}
	if string(service.caller.ProjectID) != "project-1" {
		t.Fatalf("caller project=%q want project-1", service.caller.ProjectID)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured=%T want map", result.Structured)
	}
	if structured["action"] != "delete" {
		t.Fatalf("action=%v want delete", structured["action"])
	}
	deleted, ok := structured["deleted"].(map[string]any)
	if !ok {
		t.Fatalf("deleted=%T want map", structured["deleted"])
	}
	if deleted["id"] != "dec-1" {
		t.Fatalf("deleted id=%v want dec-1", deleted["id"])
	}
}

func TestHandleDeleteRejectsMemberlessCaller(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"delete",
		"decision_id":"dec-1"
	}`))
	if err == nil || !strings.Contains(err.Error(), "decision: member_id is required") {
		t.Fatalf("err=%v want decision member_id required", err)
	}
	if service.deleteID != "" {
		t.Fatalf("decision service ran despite member-less caller: %q", service.deleteID)
	}
}

// decode() + validateActionFields() are the decision tool's deterministic-input
// gate: every other tool has a TestDecodeRejects* suite pinning its decode path;
// the decision tool had none despite the richest validation. These white-box
// tests lock each rejection branch so malformed input fails predictably instead
// of panicking or silently no-opping.

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"title":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNonStringAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":123}`))
	if err == nil || !strings.Contains(err.Error(), "action must be a string") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsUnsupportedAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"approve"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "approve"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"log","bogus":"x"}`))
	if err == nil || !strings.Contains(err.Error(), `field "bogus" is not valid for action "log"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsLogOnlyFieldForDelete(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"delete","decision_id":"dec-1","title":"x"}`))
	if err == nil || !strings.Contains(err.Error(), `field "title" is not valid for action "delete"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullDecisionIDForDelete(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"delete","decision_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "decision_id" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleDeleteRejectsMissingDecisionID(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "project-1",
		ActorMemberID: "member-1",
		UserID:        "user-1",
	}, json.RawMessage(`{"action":"delete"}`))
	if err == nil || !strings.Contains(err.Error(), "decision: decision_id is required") {
		t.Fatalf("err=%v want decision_id required", err)
	}
	if service.deleteID != "" {
		t.Fatalf("decision service ran without decision_id: %q", service.deleteID)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"log","title":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "title" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"log"`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestHandleLogRejectsProjectlessCaller completes the session-guard pair: the
// daemon can hand decision.log a session with no project bound. project_id is
// checked before member_id (handler.go:38 then :42), so a project-less caller
// must fail loudly there, before the decision service runs.
func TestHandleLogRejectsProjectlessCaller(t *testing.T) {
	service := &recordingDecisionService{}
	_, err := NewHandler().Handle(context.Background(), CallContext{
		Decisions:     service,
		ProjectID:     "",
		ActorMemberID: "member-1",
		UserID:        "user-1",
	}, json.RawMessage(`{
		"action":"log",
		"title":"Choose API labels",
		"rationale":"Users need readable labels."
	}`))
	if err == nil || !strings.Contains(err.Error(), "decision: project_id is required") {
		t.Fatalf("err=%v want decision project_id required", err)
	}
	if service.req.Title != "" {
		t.Fatalf("decision service ran despite project-less caller: %+v", service.req)
	}
}
