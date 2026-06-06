package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type stubService struct {
	createReq   taskapp.CreateTaskParams
	completeReq taskapp.CompleteTaskParams
	listReq     taskdomain.TaskFilter
	listResp    []taskdomain.Task
	assignReq   taskapp.AssignTaskParams
	reviewReq   taskapp.ReviewTaskParams
	seenCaller  taskapp.Caller
	called      string
}

func (s *stubService) capture(ctx context.Context, called string) {
	s.called = called
	resolved, _ := (caller.ContextResolver{}).ResolveCaller(ctx)
	s.seenCaller = resolved
}

func (s *stubService) Create(ctx context.Context, req taskapp.CreateTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "create")
	s.createReq = req
	return taskdomain.Task{ID: "task-1", ProjectID: req.ProjectID, AssignedTo: req.AssignedTo, AssignedToLabel: "Worker engineer", Description: req.Description, Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) Get(ctx context.Context, id taskdomain.TaskID) (taskdomain.Task, error) {
	s.capture(ctx, "get")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) List(ctx context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error) {
	s.capture(ctx, "list")
	s.listReq = filter
	if s.listResp != nil {
		return s.listResp, nil
	}
	return []taskdomain.Task{{ID: "task-1", ProjectID: filter.ProjectID, AssignedTo: filter.AssignedTo, Status: taskdomain.TaskStatusPending}}, nil
}

func (s *stubService) Claim(ctx context.Context, id taskdomain.TaskID) (taskdomain.Task, error) {
	s.capture(ctx, "claim")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", ClaimedByMemberID: "worker-1", Status: taskdomain.TaskStatusActive}, nil
}

func (s *stubService) Release(ctx context.Context, id taskdomain.TaskID) (taskdomain.Task, error) {
	s.capture(ctx, "release")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) Complete(ctx context.Context, req taskapp.CompleteTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "complete")
	s.completeReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", CreatedBy: "coord-1", AssignedTo: "worker-1", Summary: req.Summary, Metadata: req.Metadata, Status: taskdomain.TaskStatusInReview}, nil
}

func (s *stubService) Block(ctx context.Context, id taskdomain.TaskID, reason string) (taskdomain.Task, error) {
	s.capture(ctx, "block")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Error: reason, Status: taskdomain.TaskStatusBlocked}, nil
}

func (s *stubService) Unblock(ctx context.Context, id taskdomain.TaskID, note string) (taskdomain.Task, error) {
	s.capture(ctx, "unblock")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Error: note, Status: taskdomain.TaskStatusActive}, nil
}

func (s *stubService) Assign(ctx context.Context, req taskapp.AssignTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "assign")
	s.assignReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: req.AssignedTo, Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) Cancel(ctx context.Context, id taskdomain.TaskID, reason string) (taskdomain.Task, error) {
	s.capture(ctx, "cancel")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Error: reason, Status: taskdomain.TaskStatusCanceled}, nil
}

func (s *stubService) ApproveReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "approve")
	s.reviewReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Status: taskdomain.TaskStatusSucceeded}, nil
}

func (s *stubService) RetryReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "retry")
	s.reviewReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Error: req.Reason, Status: taskdomain.TaskStatusActive}, nil
}

func (s *stubService) FailReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "fail")
	s.reviewReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Error: req.Reason, Status: taskdomain.TaskStatusFailed}, nil
}

type stubMembers struct {
	members map[member.ID]member.Record
}

func (s stubMembers) GetMember(_ context.Context, id member.ID) (member.Record, error) {
	return s.members[id], nil
}

func TestHandleCreateCallsTaskService(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1","mission_ref":"mission-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "create" {
		t.Fatalf("called=%q want create", svc.called)
	}
	if svc.createReq.AssignedTo != "worker-1" || svc.createReq.Description != "ship it" || svc.createReq.ProjectID != "space-1" {
		t.Fatalf("create req = %+v", svc.createReq)
	}
	if svc.createReq.MissionRef != "mission-1" {
		t.Fatalf("missionRef=%q want mission-1", svc.createReq.MissionRef)
	}
	if svc.seenCaller.MemberID != "coord-1" {
		t.Fatalf("caller=%+v want coord-1", svc.seenCaller)
	}
	if !strings.Contains(result.Text, `"action":"create"`) {
		t.Fatalf("text = %s", result.Text)
	}
}

func TestHandleCreateAssignedToSelfReturnsClaimGuidance(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"coord-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if got := structured["nextAction"]; got != "claim" {
		t.Fatalf("nextAction=%v want claim", got)
	}
	if got := structured["guidance"]; got == "" {
		t.Fatal("guidance is empty")
	}
}

func TestHandleCreateAssignedToWorkerDoesNotReturnClaimGuidance(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if got := structured["nextAction"]; got != nil {
		t.Fatalf("nextAction=%v want omitted for worker assignment", got)
	}
}

func TestHandleCreateReturnsReadableTaskMemberLabel(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !strings.Contains(result.Text, `"assignedToLabel":"Worker engineer"`) {
		t.Fatalf("result missing assignedToLabel: %s", result.Text)
	}
}

func TestHandleListResolvesLegacyTaskMemberLabels(t *testing.T) {
	svc := &stubService{listResp: []taskdomain.Task{{
		ID:                "task-legacy",
		ProjectID:         "space-1",
		AssignedTo:        "worker-1",
		ClaimedByMemberID: "worker-1",
		CreatedBy:         "coord-1",
		Status:            taskdomain.TaskStatusPending,
	}}}

	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"list","limit":10}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !strings.Contains(result.Text, `"assignedToLabel":"Worker"`) {
		t.Fatalf("result missing resolved assignedToLabel: %s", result.Text)
	}
	if !strings.Contains(result.Text, `"claimedByMemberLabel":"Worker"`) {
		t.Fatalf("result missing resolved claimedByMemberLabel: %s", result.Text)
	}
	if !strings.Contains(result.Text, `"createdByLabel":"Coordinator"`) {
		t.Fatalf("result missing resolved createdByLabel: %s", result.Text)
	}
}

func TestHandleListPrefersStampedTaskMemberLabels(t *testing.T) {
	svc := &stubService{listResp: []taskdomain.Task{{
		ID:                   "task-stamped",
		ProjectID:            "space-1",
		AssignedTo:           "worker-1",
		AssignedToLabel:      "Original assignee",
		ClaimedByMemberID:    "worker-1",
		ClaimedByMemberLabel: "Original claimant",
		CreatedBy:            "coord-1",
		CreatedByLabel:       "Original creator",
		Status:               taskdomain.TaskStatusPending,
	}}}

	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"list","limit":10}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, want := range []string{
		`"assignedToLabel":"Original assignee"`,
		`"claimedByMemberLabel":"Original claimant"`,
		`"createdByLabel":"Original creator"`,
	} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("result missing stamped label %s: %s", want, result.Text)
		}
	}
	if strings.Contains(result.Text, `"assignedToLabel":"Worker"`) {
		t.Fatalf("result overwrote stamped assigned label: %s", result.Text)
	}
}

func TestHandleWorkerListScopesToAssignedMember(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "worker-1"), json.RawMessage(`{"action":"list","limit":10}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "list" {
		t.Fatalf("called=%q want list", svc.called)
	}
	if svc.listReq.AssignedTo != "worker-1" || svc.listReq.ProjectID != "space-1" || svc.listReq.Limit != 10 {
		t.Fatalf("list req = %+v", svc.listReq)
	}
}

func TestHandleRequiresRegisteredMemberID(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, ""), json.RawMessage(`{"action":"list","limit":10}`))
	if err == nil {
		t.Fatal("expected missing registered member error")
	}
	if !strings.Contains(err.Error(), "registered member_id is required") {
		t.Fatalf("error=%q", err)
	}
	if strings.Contains(err.Error(), "caller member") {
		t.Fatalf("error leaked internal caller wording: %q", err)
	}
}

func TestHandleSubmitSelfReviewReturnsReviewGuidance(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"submit","task_id":"task-1","summary":"done"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if got := structured["nextAction"]; got != "review" {
		t.Fatalf("nextAction=%v want review", got)
	}
	if got := structured["guidance"]; got == "" {
		t.Fatal("guidance is empty")
	}
}

func TestHandleSubmitAcceptsStringMetadata(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"submit","task_id":"task-1","summary":"done","metadata":{"commit":"abc123","decision":"dec-1"}}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.completeReq.Metadata["commit"] != "abc123" || svc.completeReq.Metadata["decision"] != "dec-1" {
		t.Fatalf("complete metadata=%+v", svc.completeReq.Metadata)
	}
	if !strings.Contains(result.Text, `"metadata":{"commit":"abc123","decision":"dec-1"}`) {
		t.Fatalf("result missing metadata: %s", result.Text)
	}
}

func TestHandleReassignCallsAssign(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"reassign","task_id":"task-1","assignee_member_id":"worker-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "assign" {
		t.Fatalf("called=%q want assign", svc.called)
	}
	if svc.assignReq.TaskID != "task-1" || svc.assignReq.AssignedTo != "worker-1" {
		t.Fatalf("assign req = %+v", svc.assignReq)
	}
}

func TestHandleCancelReturnsStatusReason(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"cancel","task_id":"task-1","reason":"no longer needed"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if strings.Contains(result.Text, `"error"`) {
		t.Fatalf("cancel result should not expose reason as error: %s", result.Text)
	}
	if !strings.Contains(result.Text, `"statusReason":"no longer needed"`) {
		t.Fatalf("cancel result missing statusReason: %s", result.Text)
	}
}

func TestHandleReviewMapsDecisionToServiceMethod(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"retry","reason":"tighten it","criteria":[{"id":"criterion-1","satisfied":false}]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "retry" {
		t.Fatalf("called=%q want retry", svc.called)
	}
	if svc.reviewReq.TaskID != "task-1" || svc.reviewReq.Reason != "tighten it" {
		t.Fatalf("review req = %+v", svc.reviewReq)
	}
	if len(svc.reviewReq.Criteria) != 1 || svc.reviewReq.Criteria[0].ID != "criterion-1" || svc.reviewReq.Criteria[0].Satisfied {
		t.Fatalf("criteria = %+v", svc.reviewReq.Criteria)
	}
}

func TestHandleReviewAcceptsApproveSummaryAlias(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"approve","summary":"approved after inspection","criteria":[{"id":"criterion-1","satisfied":true}]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "approve" {
		t.Fatalf("called=%q want approve", svc.called)
	}
	if svc.reviewReq.Reason != "approved after inspection" {
		t.Fatalf("review reason=%q want summary alias", svc.reviewReq.Reason)
	}
}

func TestHandleReviewUsesSummaryAndNoteAsFeedbackFallbacks(t *testing.T) {
	t.Run("summary", func(t *testing.T) {
		svc := &stubService{}
		_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"retry","summary":"tighten summary","criteria":[{"id":"criterion-1","satisfied":false}]}`))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if svc.called != "retry" || svc.reviewReq.Reason != "tighten summary" {
			t.Fatalf("called=%q review req=%+v", svc.called, svc.reviewReq)
		}
	})
	t.Run("note", func(t *testing.T) {
		svc := &stubService{}
		_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"fail","note":"missing evidence","criteria":[{"id":"criterion-1","satisfied":false}]}`))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if svc.called != "fail" || svc.reviewReq.Reason != "missing evidence" {
			t.Fatalf("called=%q review req=%+v", svc.called, svc.reviewReq)
		}
	})
}

func TestDecodeRejectsUnknownAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"force_complete"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsLegacyField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"create","goal":"old shape"}`))
	if err == nil || !strings.Contains(err.Error(), `field "goal" is not valid for action "create"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list","status":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "status" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsFieldFromOtherAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list","summary":"done"}`))
	if err == nil || !strings.Contains(err.Error(), `field "summary" is not valid for action "list"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNonStringMetadata(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1","metadata":{"smoke":true}}`))
	if err == nil || !strings.Contains(err.Error(), "metadata must be an object with string values") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeAcceptsStringMetadata(t *testing.T) {
	input, err := decode(json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1","metadata":{"smoke":"true"}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if input.Metadata["smoke"] != "true" {
		t.Fatalf("metadata=%+v", input.Metadata)
	}
}

func TestDecodeAcceptsSubmitStringMetadata(t *testing.T) {
	input, err := decode(json.RawMessage(`{"action":"submit","task_id":"task-1","summary":"done","metadata":{"commit":"abc123"}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if input.Metadata["commit"] != "abc123" {
		t.Fatalf("metadata=%+v", input.Metadata)
	}
}

func TestContextWithSessionActorStampsMemberCaller(t *testing.T) {
	ctx := contextWithSessionActor(context.Background(), "member-1", "space-1")
	resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
	if err != nil {
		t.Fatalf("ResolveCaller: %v", err)
	}
	if resolved.MemberID != "member-1" || resolved.ProjectID != "space-1" {
		t.Fatalf("caller=%+v want member-1 in space-1", resolved)
	}
}

func TestSchemaExcludesPauseAndResume(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	props := schema["properties"].(map[string]any)
	action := props["action"].(map[string]any)
	enums := action["enum"].([]any)
	for _, value := range enums {
		if value == "pause" || value == "resume" {
			t.Fatalf("schema exposes removed action %q", value)
		}
	}
	if _, ok := props["assignee_member_id"]; !ok {
		t.Fatalf("schema missing assignee_member_id")
	}
}

func TestSchemaDocumentsStringMetadata(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, ok := schema["allOf"]; ok {
		t.Fatalf("schema must not use top-level allOf: %+v", schema["allOf"])
	}
	props := schema["properties"].(map[string]any)
	metadata := props["metadata"].(map[string]any)
	additional := metadata["additionalProperties"].(map[string]any)
	if additional["type"] != "string" {
		t.Fatalf("metadata additionalProperties=%+v want string", additional)
	}
	if _, ok := metadata["anyOf"]; ok {
		t.Fatalf("metadata should not be nullable: %+v", metadata)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "action" {
		t.Fatalf("required=%v want only action", required)
	}
}

func callContext(svc *stubService, actorMemberID string) CallContext {
	return CallContext{
		Tasks:         svc,
		Members:       stubMembers{members: members()},
		ProjectID:     "space-1",
		ActorMemberID: actorMemberID,
	}
}

func members() map[member.ID]member.Record {
	return map[member.ID]member.Record{
		"coord-1":  {ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive, DisplayName: "Coordinator"},
		"worker-1": {ID: "worker-1", ProjectID: "space-1", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive, DisplayName: "Worker"},
	}
}
