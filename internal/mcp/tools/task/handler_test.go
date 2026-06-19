package task

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type stubService struct {
	createReq   taskapp.CreateTaskParams
	completeReq taskapp.CompleteTaskParams
	listReq     taskdomain.TaskFilter
	listResp    []taskdomain.Task
	assignReq   taskapp.AssignTaskParams
	updateReq   taskapp.UpdateTaskParams
	reviewReq   taskapp.ReviewTaskParams
	reviewErr   error
	attachRef   string
	attachErr   error
	getResp     *taskdomain.Task
	getErr      error
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
	return taskdomain.Task{ID: "task-1", ProjectID: req.ProjectID, AssignedTo: taskdomain.MemberIDFromString(req.AssignedTo), AssignedToLabel: "Worker engineer", Description: req.Description, Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) Get(ctx context.Context, id taskdomain.TaskID) (taskdomain.Task, error) {
	s.capture(ctx, "get")
	if s.getErr != nil {
		return taskdomain.Task{}, s.getErr
	}
	if s.getResp != nil {
		return *s.getResp, nil
	}
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
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: taskdomain.MemberIDFromString(req.AssignedTo), Status: taskdomain.TaskStatusPending}, nil
}

func (s *stubService) Cancel(ctx context.Context, id taskdomain.TaskID, reason string) (taskdomain.Task, error) {
	s.capture(ctx, "cancel")
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Error: reason, Status: taskdomain.TaskStatusCanceled}, nil
}

func (s *stubService) Update(ctx context.Context, req taskapp.UpdateTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "update")
	s.updateReq = req
	title := ""
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
	}
	description := ""
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
	}
	taskKind := ""
	if req.TaskKind != nil {
		taskKind = strings.TrimSpace(*req.TaskKind)
	}
	keyResultRef := ""
	if req.KeyResultRef != nil {
		keyResultRef = strings.TrimSpace(*req.KeyResultRef)
	}
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Title: title, Description: description, TaskKind: taskKind, KeyResultRef: keyResultRef, Metadata: req.Metadata, Status: taskdomain.TaskStatusPending}, nil

}

func (s *stubService) ApproveReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "approve")
	s.reviewReq = req
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Status: taskdomain.TaskStatusSucceeded}, nil
}

func (s *stubService) RetryReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "retry")
	s.reviewReq = req
	if s.reviewErr != nil {
		return taskdomain.Task{}, s.reviewErr
	}
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Error: req.Reason, Status: taskdomain.TaskStatusActive}, nil
}

func (s *stubService) FailReview(ctx context.Context, req taskapp.ReviewTaskParams) (taskdomain.Task, error) {
	s.capture(ctx, "fail")
	s.reviewReq = req
	if s.reviewErr != nil {
		return taskdomain.Task{}, s.reviewErr
	}
	return taskdomain.Task{ID: req.TaskID, ProjectID: "space-1", AssignedTo: "worker-1", Error: req.Reason, Status: taskdomain.TaskStatusFailed}, nil
}

func (s *stubService) AttachArtifact(ctx context.Context, id taskdomain.TaskID, ref string) (taskdomain.Task, error) {
	s.capture(ctx, "attach")
	s.attachRef = ref
	if s.attachErr != nil {
		return taskdomain.Task{}, s.attachErr
	}
	return taskdomain.Task{ID: id, ProjectID: "space-1", AssignedTo: "worker-1", Artifacts: []string{ref}, Status: taskdomain.TaskStatusActive}, nil
}

type stubFileStore struct {
	uploads   []fileapp.UploadInput
	deletes   []fileapp.PathInput
	uploadErr error
}

func (s *stubFileStore) Upload(_ context.Context, input fileapp.UploadInput) (fileapp.PathResult, error) {
	s.uploads = append(s.uploads, input)
	if s.uploadErr != nil {
		return fileapp.PathResult{}, s.uploadErr
	}
	return fileapp.PathResult{Path: input.Path}, nil
}

func (s *stubFileStore) Delete(_ context.Context, input fileapp.PathInput) (struct{}, error) {
	s.deletes = append(s.deletes, input)
	return struct{}{}, nil
}

type stubMembers struct {
	members map[member.ID]member.Record
}

func (s stubMembers) GetMember(_ context.Context, id member.ID) (member.Record, error) {
	return s.members[id], nil
}

// membersFunc adapts a function to the members lookup port. The map-backed
// stubMembers always succeeds (a missing id returns the zero Record, not an
// error), so it cannot express a roster load failure or a per-id error. The
// actor and assignee load-error guards need exactly those shapes.
type membersFunc func(context.Context, member.ID) (member.Record, error)

func (f membersFunc) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	return f(ctx, id)
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

func TestHandleCreateReturnsUltraLeanAck(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"create","description":"ship it","assignee_member_id":"worker-1"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// A mutation ack is just {id, status} — the caller already holds everything
	// else. No assignee echo, no labels, no title/refs/kind, no description.
	if !strings.Contains(result.Text, `"id":"task-1"`) || !strings.Contains(result.Text, `"status":"pending"`) {
		t.Fatalf("create ack missing id/status: %s", result.Text)
	}
	for _, unwanted := range []string{`"assignee"`, `Label`, `"assignedToMemberId"`, `"title"`, `"description"`, `"keyResultRef"`} {
		if strings.Contains(result.Text, unwanted) {
			t.Fatalf("create ack should not contain %q: %s", unwanted, result.Text)
		}
	}
}

func TestHandleListReturnsLeanRows(t *testing.T) {
	svc := &stubService{listResp: []taskdomain.Task{{
		ID:                "task-1",
		ProjectID:         "space-1",
		AssignedTo:        "worker-1",
		ClaimedByMemberID: "worker-1",
		CreatedBy:         "coord-1",
		Description:       "a long description that should not appear in list rows",
		Status:            taskdomain.TaskStatusPending,
	}}}

	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"list","limit":10}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// Lean rows: id + status + member ids, but no labels and no description.
	if !strings.Contains(result.Text, `"id":"task-1"`) || !strings.Contains(result.Text, `"assignedToMemberId":"worker-1"`) {
		t.Fatalf("list row missing id/assignee: %s", result.Text)
	}
	for _, unwanted := range []string{`Label":`, "a long description"} {
		if strings.Contains(result.Text, unwanted) {
			t.Fatalf("lean list row should not contain %q: %s", unwanted, result.Text)
		}
	}
}

func TestHandleMemberListDefaultsToProjectScope(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "worker-1"), json.RawMessage(`{"action":"list","limit":10}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "list" {
		t.Fatalf("called=%q want list", svc.called)
	}
	if svc.listReq.AssignedTo != "" || svc.listReq.ProjectID != "space-1" || svc.listReq.Limit != 10 {
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

// actor() and assignee() are the task tool's authorization layer: both run
// before any task-service call, so a caller or assignee that is unloadable,
// inactive, or from another project must be rejected with the service never
// touched. Only the member-less actor path was covered. The tests below pin
// the remaining failure paths; each asserts svc.called stays "" — proof the
// guard fired before dispatch reached the service.

func TestHandleRejectsActorLoadError(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks:         svc,
		Members:       membersFunc(func(context.Context, member.ID) (member.Record, error) { return member.Record{}, member.ErrNotFound }),
		ProjectID:     "space-1",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"list","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), "load registered member") {
		t.Fatalf("error=%v want load registered member", err)
	}
	if !errors.Is(err, member.ErrNotFound) {
		t.Fatalf("error does not wrap member.ErrNotFound: %v", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite unloadable actor: %q", svc.called)
	}
}

func TestHandleRejectsInactiveActor(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks: svc,
		Members: stubMembers{members: map[member.ID]member.Record{
			"coord-1": {ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleRemoved},
		}},
		ProjectID:     "space-1",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"list","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), `registered member "coord-1" is not active`) {
		t.Fatalf("error=%v want inactive actor", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite inactive actor: %q", svc.called)
	}
}

func TestHandleRejectsCrossProjectAssignee(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks: svc,
		Members: stubMembers{members: map[member.ID]member.Record{
			"coord-1": {ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive},
			"alien-1": {ID: "alien-1", ProjectID: "space-2", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive},
		}},
		ProjectID:     "space-1",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","description":"x","assignee_member_id":"alien-1","mission_ref":"m-1"}`))
	if err == nil || !strings.Contains(err.Error(), "belongs to project") {
		t.Fatalf("error=%v want cross-project assignee", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite cross-project assignee: %q", svc.called)
	}
}

func TestHandleRejectsProjectMismatchInActorContext(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks: svc,
		Members: stubMembers{members: map[member.ID]member.Record{
			"coord-1": {ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive},
		}},
		ProjectID:     "space-2",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"list","limit":10}`))
	if err == nil || !strings.Contains(err.Error(), "registered member \"coord-1\" is not in project \"space-2\"") {
		t.Fatalf("error=%v want project mismatch for actor context", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite actor context mismatch: %q", svc.called)
	}
}

func TestHandleRejectsInactiveAssignee(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks: svc,
		Members: stubMembers{members: map[member.ID]member.Record{
			"coord-1": {ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive},
			"dead-1":  {ID: "dead-1", ProjectID: "space-1", MemberType: member.TypeWorker, LifecycleState: member.LifecycleRemoved},
		}},
		ProjectID:     "space-1",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","description":"x","assignee_member_id":"dead-1","mission_ref":"m-1"}`))
	if err == nil || !strings.Contains(err.Error(), `assignee member "dead-1" is not active`) {
		t.Fatalf("error=%v want inactive assignee", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite inactive assignee: %q", svc.called)
	}
}

func TestHandleRejectsAssigneeLoadError(t *testing.T) {
	svc := &stubService{}
	call := CallContext{
		Tasks: svc,
		Members: membersFunc(func(_ context.Context, id member.ID) (member.Record, error) {
			if id == "coord-1" {
				return member.Record{ID: "coord-1", ProjectID: "space-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
			}
			return member.Record{}, member.ErrNotFound
		}),
		ProjectID:     "space-1",
		ActorMemberID: "coord-1",
	}
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","description":"x","assignee_member_id":"ghost-1","mission_ref":"m-1"}`))
	if err == nil || !strings.Contains(err.Error(), "load assignee member") {
		t.Fatalf("error=%v want load assignee member", err)
	}
	if !errors.Is(err, member.ErrNotFound) {
		t.Fatalf("error does not wrap member.ErrNotFound: %v", err)
	}
	if svc.called != "" {
		t.Fatalf("task service ran despite unloadable assignee: %q", svc.called)
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
	// The lean mutation response no longer echoes metadata back to the model.
	if strings.Contains(result.Text, `"metadata"`) {
		t.Fatalf("lean submit response should not echo metadata: %s", result.Text)
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

func TestHandleUpdateCallsUpdateAndPassesMissionRef(t *testing.T) {
	svc := &stubService{}
	result, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"update","task_id":"task-1","title":"Tighten checks","task_kind":"security_scan","key_result_ref":"kr-1","mission_ref":"mission-1","metadata":{"tag":"review"},"acceptance_criteria":["check logs","patch handler"]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "update" {
		t.Fatalf("called=%q want update", svc.called)
	}
	if svc.updateReq.TaskID != "task-1" || svc.updateReq.TaskKind == nil || *svc.updateReq.TaskKind != "security_scan" || svc.updateReq.KeyResultRef == nil || *svc.updateReq.KeyResultRef != "kr-1" {
		t.Fatalf("update req=%+v", svc.updateReq)
	}
	if svc.updateReq.Metadata["missionRef"] != "mission-1" || svc.updateReq.Metadata["tag"] != "review" {
		t.Fatalf("update metadata=%+v", svc.updateReq.Metadata)
	}
	if svc.updateReq.Title == nil || *svc.updateReq.Title != "Tighten checks" {
		t.Fatalf("update title=%v", svc.updateReq.Title)
	}
	if svc.updateReq.AcceptanceCriteria == nil || len(*svc.updateReq.AcceptanceCriteria) != 2 {
		t.Fatalf("update acceptance criteria=%+v", svc.updateReq.AcceptanceCriteria)
	}
	if criterion := (*svc.updateReq.AcceptanceCriteria)[0]; criterion.ID == "" || criterion.Text == "" {
		t.Fatalf("first acceptance criterion=%+v", criterion)
	}
	if svc.updateReq.Description != nil {
		t.Fatalf("unexpected description=%v", *svc.updateReq.Description)
	}
	if !strings.Contains(result.Text, `"action":"update"`) {
		t.Fatalf("text = %s", result.Text)
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

func TestHandleReviewPassesReviewFieldsForApprove(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"approve","summary":"approved after inspection","note":"all criteria met","criteria":[{"id":"criterion-1","satisfied":true}]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "approve" {
		t.Fatalf("called=%q want approve", svc.called)
	}
	if svc.reviewReq.Reason != "" {
		t.Fatalf("review reason=%q want empty", svc.reviewReq.Reason)
	}
	if svc.reviewReq.Summary != "approved after inspection" {
		t.Fatalf("review summary=%q want approved after inspection", svc.reviewReq.Summary)
	}
	if svc.reviewReq.Note != "all criteria met" {
		t.Fatalf("review note=%q want all criteria met", svc.reviewReq.Note)
	}
}

func TestHandleReviewPassesReviewFieldsForRetryAndFail(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"retry","reason":"tighten it","summary":"retry summary","note":"add additional unit tests","criteria":[{"id":"criterion-1","satisfied":false}]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "retry" {
		t.Fatalf("called=%q want retry", svc.called)
	}
	if svc.reviewReq.Reason != "tighten it" {
		t.Fatalf("review reason=%q want tighten it", svc.reviewReq.Reason)
	}
	if svc.reviewReq.Summary != "retry summary" {
		t.Fatalf("review summary=%q want retry summary", svc.reviewReq.Summary)
	}
	if svc.reviewReq.Note != "add additional unit tests" {
		t.Fatalf("review note=%q want add additional unit tests", svc.reviewReq.Note)
	}

	svc = &stubService{}
	_, err = NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"fail","reason":"security risk","summary":"blocked due risk","note":"update validation","criteria":[{"id":"criterion-1","satisfied":false}]}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "fail" {
		t.Fatalf("called=%q want fail", svc.called)
	}
	if svc.reviewReq.Reason != "security risk" {
		t.Fatalf("review reason=%q want security risk", svc.reviewReq.Reason)
	}
	if svc.reviewReq.Summary != "blocked due risk" {
		t.Fatalf("review summary=%q want blocked due risk", svc.reviewReq.Summary)
	}
	if svc.reviewReq.Note != "update validation" {
		t.Fatalf("review note=%q want update validation", svc.reviewReq.Note)
	}
}

func TestHandleReviewRetryFailRequireReason(t *testing.T) {
	svc := &stubService{}
	_, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"retry","summary":"needs more detail","criteria":[{"id":"criterion-1","satisfied":false}]}`))
	if err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected reason requirement, got %v", err)
	}
	if svc.called != "" {
		t.Fatalf("service called without reason: %q", svc.called)
	}

	_, err = NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"fail","note":"needs more evidence","criteria":[{"id":"criterion-1","satisfied":false}]}`))
	if err == nil || !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected reason requirement, got %v", err)
	}
	if svc.called != "" {
		t.Fatalf("service called without reason: %q", svc.called)
	}
}

func callContextWithFiles(svc *stubService, files *stubFileStore, actorMemberID string) CallContext {
	call := callContext(svc, actorMemberID)
	call.Files = files
	return call
}

func TestHandleAttachUploadsAndAppendsArtifact(t *testing.T) {
	svc := &stubService{}
	files := &stubFileStore{}
	pngB64 := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n fake image body"))
	res, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), json.RawMessage(`{"action":"attach","task_id":"task-1","file_name":"build-screenshot.png","content_b64":"`+pngB64+`"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(files.uploads) != 1 {
		t.Fatalf("uploads=%d want 1", len(files.uploads))
	}
	upload := files.uploads[0]
	if upload.Path != "/project/.agen8/attachments/task-1/build-screenshot.png" {
		t.Fatalf("upload path=%q", upload.Path)
	}
	if upload.BytesB64 != pngB64 || upload.Content != "" {
		t.Fatalf("upload bytes not forwarded verbatim: %+v", upload)
	}
	if string(upload.ProjectID) != "space-1" {
		t.Fatalf("upload projectID=%q want space-1", upload.ProjectID)
	}
	wantRef := "file:/project/.agen8/attachments/task-1/build-screenshot.png"
	if svc.attachRef != wantRef {
		t.Fatalf("attach ref=%q want %q", svc.attachRef, wantRef)
	}
	structured, ok := res.Structured.(map[string]any)
	if !ok || structured["artifactRef"] != wantRef {
		t.Fatalf("result missing artifactRef: %+v", res.Structured)
	}
	if len(files.deletes) != 0 {
		t.Fatalf("unexpected cleanup delete: %+v", files.deletes)
	}
}

func TestHandleAttachRejectsTraversalAndPathFileNames(t *testing.T) {
	for _, name := range []string{"../../etc/passwd", "nested/dir.png", `back\slash.png`, "..", "."} {
		svc := &stubService{}
		files := &stubFileStore{}
		body, _ := json.Marshal(map[string]any{"action": "attach", "task_id": "task-1", "file_name": name, "content": "x"})
		_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), body)
		if err == nil || !strings.Contains(err.Error(), "file_name") {
			t.Fatalf("file_name=%q: expected rejection, got %v", name, err)
		}
		if len(files.uploads) != 0 {
			t.Fatalf("file_name=%q: upload happened despite rejection", name)
		}
	}
}

func TestHandleAttachRequiresExactlyOneContentField(t *testing.T) {
	for _, payload := range []string{
		`{"action":"attach","task_id":"task-1","file_name":"a.txt"}`,
		`{"action":"attach","task_id":"task-1","file_name":"a.txt","content":"x","content_b64":"eA=="}`,
		`{"action":"attach","task_id":"task-1","file_name":"a.txt","content":"x","file_path":"/tmp/a.txt"}`,
		`{"action":"attach","task_id":"task-1","content_b64":"eA==","file_path":"/tmp/a.txt"}`,
	} {
		svc := &stubService{}
		files := &stubFileStore{}
		_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), json.RawMessage(payload))
		if err == nil || !strings.Contains(err.Error(), "exactly one of content, content_b64, or file_path") {
			t.Fatalf("payload=%s: expected exactly-one error, got %v", payload, err)
		}
		if len(files.uploads) != 0 {
			t.Fatalf("payload=%s: upload happened despite rejection", payload)
		}
	}
}

func TestHandleAttachByFilePathReadsBytesAndDefaultsFileName(t *testing.T) {
	dir := t.TempDir()
	body := []byte("\x89PNG\r\n\x1a\n fake image body")
	path := filepath.Join(dir, "pulse-after.png")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	svc := &stubService{}
	files := &stubFileStore{}
	payload, _ := json.Marshal(map[string]any{"action": "attach", "task_id": "task-1", "file_path": path})
	res, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), payload)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(files.uploads) != 1 {
		t.Fatalf("uploads=%d want 1", len(files.uploads))
	}
	upload := files.uploads[0]
	if upload.Path != "/project/.agen8/attachments/task-1/pulse-after.png" {
		t.Fatalf("upload path=%q (file_name should default to base name)", upload.Path)
	}
	if upload.BytesB64 != base64.StdEncoding.EncodeToString(body) {
		t.Fatalf("uploaded bytes do not match the source file")
	}
	structured, ok := res.Structured.(map[string]any)
	if !ok || structured["artifactRef"] != "file:/project/.agen8/attachments/task-1/pulse-after.png" {
		t.Fatalf("result missing artifactRef: %+v", res.Structured)
	}
	// Copy semantics: the source file must still exist untouched.
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(body) {
		t.Fatalf("source file was modified or removed: err=%v", err)
	}
}

func TestHandleAttachByFilePathAllowsFiveMegabytes(t *testing.T) {
	dir := t.TempDir()
	body := []byte(strings.Repeat("x", 5<<20))
	path := filepath.Join(dir, "five-megabytes.bin")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	svc := &stubService{}
	files := &stubFileStore{}
	payload, _ := json.Marshal(map[string]any{"action": "attach", "task_id": "task-1", "file_path": path})
	if _, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(files.uploads) != 1 {
		t.Fatalf("uploads=%d want 1", len(files.uploads))
	}
	decoded, err := base64.StdEncoding.DecodeString(files.uploads[0].BytesB64)
	if err != nil {
		t.Fatalf("decode upload bytes: %v", err)
	}
	if len(decoded) != len(body) {
		t.Fatalf("uploaded bytes=%d want %d", len(decoded), len(body))
	}
	if files.uploads[0].Path != "/project/.agen8/attachments/task-1/five-megabytes.bin" {
		t.Fatalf("upload path=%q", files.uploads[0].Path)
	}
}

func TestHandleAttachByFilePathExplicitFileNameWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "raw-capture.png")
	if err := os.WriteFile(path, []byte("img"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	svc := &stubService{}
	files := &stubFileStore{}
	payload, _ := json.Marshal(map[string]any{"action": "attach", "task_id": "task-1", "file_path": path, "file_name": "verification.png"})
	if _, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), payload); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(files.uploads) != 1 || files.uploads[0].Path != "/project/.agen8/attachments/task-1/verification.png" {
		t.Fatalf("explicit file_name not honored: %+v", files.uploads)
	}
}

func TestHandleAttachByFilePathGuards(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("create big fixture: %v", err)
	}
	if err := f.Truncate(maxAttachmentFileBytes + 1); err != nil {
		t.Fatalf("truncate big fixture: %v", err)
	}
	_ = f.Close()

	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"relative path", "relative/capture.png", "absolute"},
		{"missing file", filepath.Join(dir, "nope.png"), "file_path"},
		{"directory", dir, "regular file"},
		{"over size cap", big, "attachment limit"},
	}
	for _, tc := range cases {
		svc := &stubService{}
		files := &stubFileStore{}
		payload, _ := json.Marshal(map[string]any{"action": "attach", "task_id": "task-1", "file_path": tc.path})
		_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), payload)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: expected error containing %q, got %v", tc.name, tc.wantErr, err)
		}
		if len(files.uploads) != 0 {
			t.Fatalf("%s: upload happened despite rejection", tc.name)
		}
	}
}

func TestHandleAttachRefusesCanceledTaskBeforeWriting(t *testing.T) {
	svc := &stubService{getResp: &taskdomain.Task{ID: "task-1", ProjectID: "space-1", Status: taskdomain.TaskStatusCanceled}}
	files := &stubFileStore{}
	_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), json.RawMessage(`{"action":"attach","task_id":"task-1","file_name":"a.txt","content":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected canceled rejection, got %v", err)
	}
	if len(files.uploads) != 0 {
		t.Fatalf("file was written for a canceled task: %+v", files.uploads)
	}
}

func TestHandleAttachRefusesNonexistentTaskBeforeWriting(t *testing.T) {
	svc := &stubService{getErr: errors.New("task task-missing not found")}
	files := &stubFileStore{}
	_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), json.RawMessage(`{"action":"attach","task_id":"task-missing","file_name":"a.txt","content":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
	if len(files.uploads) != 0 {
		t.Fatalf("file was written for a nonexistent task: %+v", files.uploads)
	}
}

func TestHandleAttachCleansUpUploadWhenAppendFails(t *testing.T) {
	svc := &stubService{attachErr: errors.New("append rejected")}
	files := &stubFileStore{}
	_, err := NewHandler().Handle(context.Background(), callContextWithFiles(svc, files, "coord-1"), json.RawMessage(`{"action":"attach","task_id":"task-1","file_name":"a.txt","content":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "append rejected") {
		t.Fatalf("expected append error to surface, got %v", err)
	}
	if len(files.uploads) != 1 {
		t.Fatalf("uploads=%d want 1", len(files.uploads))
	}
	if len(files.deletes) != 1 || files.deletes[0].Path != "/project/.agen8/attachments/task-1/a.txt" {
		t.Fatalf("expected cleanup delete of the uploaded file, got %+v", files.deletes)
	}
}

// Regression guard for the SA4006 shadow fix in the review action: a `:=` on
// the reason validation would shadow the outer err, so RetryReview/FailReview
// errors would be swallowed and the review reported as success.
func TestHandleReviewRetryFailSurfaceServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		decision string
		errMsg   string
	}{
		{decision: "retry", errMsg: "retry review rejected by service"},
		{decision: "fail", errMsg: "fail review rejected by service"},
	} {
		svc := &stubService{reviewErr: errors.New(tc.errMsg)}
		res, err := NewHandler().Handle(context.Background(), callContext(svc, "coord-1"), json.RawMessage(`{"action":"review","task_id":"task-1","decision":"`+tc.decision+`","reason":"does not meet the bar","criteria":[{"id":"criterion-1","satisfied":false}]}`))
		if svc.called != tc.decision {
			t.Fatalf("decision=%s: service called=%q want %q", tc.decision, svc.called, tc.decision)
		}
		if err == nil || !strings.Contains(err.Error(), tc.errMsg) {
			t.Fatalf("decision=%s: service error not surfaced, err=%v result=%+v", tc.decision, err, res)
		}
		if res.Text != "" || res.Structured != nil {
			t.Fatalf("decision=%s: expected empty result alongside error, got %+v", tc.decision, res)
		}
	}
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

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"description":"x"}`))
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

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list"`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestDecodeRejectsTrailingJSON feeds trailing JSON after a valid object. The
// json.Unmarshal-based validateActionFields runs first and rejects any trailing
// token, so the input surfaces as the generic "invalid arguments" error and never
// reaches a trailing-JSON guard. This proves the old guard was unreachable; it was
// removed as dead code (see dec-21debbd9). Trailing input is still rejected loudly.
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list"} {}`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
	if strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing-JSON guard was expected unreachable, but its message surfaced: %v", err)
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
