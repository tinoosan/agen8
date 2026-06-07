package mission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

var missionTestTime = time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

type stubMissionService struct {
	called      string
	createReq   missionapp.CreateMissionParams
	listProject string
	listFilter  missiondomain.MissionFilter
	updateReq   missionapp.UpdateMissionParams
	krCreateReq missionapp.CreateKeyResultParams
	deleteKRReq missionapp.DeleteKeyResultParams
	reopenKRReq missionapp.ReopenKeyResultParams
	progressReq missionapp.UpdateProgressParams
	seenCaller  caller.Caller
}

func (s *stubMissionService) capture(ctx context.Context, called string) {
	s.called = called
	resolved, _ := (caller.ContextResolver{}).ResolveCaller(ctx)
	s.seenCaller = resolved
}

func (s *stubMissionService) CreateMission(ctx context.Context, req missionapp.CreateMissionParams) (missiondomain.Mission, error) {
	s.capture(ctx, "create")
	s.createReq = req
	return missiondomain.Mission{ID: req.ID, ProjectID: req.ProjectID, Title: req.Title, Description: req.Description, Status: missiondomain.MissionStatusDraft, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) GetMission(ctx context.Context, id missiondomain.MissionID) (missiondomain.Mission, error) {
	s.capture(ctx, "get")
	return missiondomain.Mission{ID: id, ProjectID: "project-1", Title: "Mission", Status: missiondomain.MissionStatusActive, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) ListMissions(ctx context.Context, projectID string, filter missiondomain.MissionFilter) ([]missiondomain.Mission, error) {
	s.capture(ctx, "list")
	s.listProject = projectID
	s.listFilter = filter
	return []missiondomain.Mission{{ID: "mission-1", ProjectID: projectID, Title: "Mission", Status: missiondomain.MissionStatusActive, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}}, nil
}

func (s *stubMissionService) UpdateMission(ctx context.Context, req missionapp.UpdateMissionParams) (missiondomain.Mission, error) {
	s.capture(ctx, "update")
	s.updateReq = req
	status := missiondomain.MissionStatusActive
	if req.Status != nil {
		status = *req.Status
	}
	return missiondomain.Mission{ID: req.MissionID, ProjectID: "project-1", Title: "Mission", Status: status, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) DeleteMission(ctx context.Context, req missionapp.DeleteMissionParams) (missiondomain.Mission, error) {
	s.capture(ctx, "archive")
	return missiondomain.Mission{ID: req.MissionID, ProjectID: "project-1", Title: "Mission", Status: missiondomain.MissionStatusArchived, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) GetLifecycleHistory(ctx context.Context, id missiondomain.MissionID, filter missionapp.LifecycleHistoryFilter) (missionapp.LifecycleHistory, error) {
	s.capture(ctx, "history")
	return missionapp.LifecycleHistory{
		MissionID: id,
		Count:     1,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
		Entries: []missionapp.LifecycleHistoryEntry{{
			EventID:   "event-1",
			MissionID: id,
			Type:      string(missionapp.MissionEventActivated),
			Action:    "activate",
			Status:    "active",
			Note:      "ready",
			Origin:    "mission",
			Timestamp: missionTestTime,
		}},
	}, nil
}

func (s *stubMissionService) CreateKeyResult(ctx context.Context, req missionapp.CreateKeyResultParams) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_create")
	s.krCreateReq = req
	return krdomain.KeyResult{ID: req.ID, MissionID: req.MissionID, Title: req.Title, MeasurementType: req.MeasurementType, Direction: req.Direction, TargetValue: req.TargetValue, Status: krdomain.KeyResultStatusOpen, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) GetKeyResult(ctx context.Context, id krdomain.KeyResultID) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_get")
	return krdomain.KeyResult{ID: id, MissionID: "mission-1", Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, Status: krdomain.KeyResultStatusOpen, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) ListKeyResults(ctx context.Context, id missiondomain.MissionID) ([]krdomain.KeyResult, error) {
	s.capture(ctx, "kr_list")
	return []krdomain.KeyResult{{ID: "kr-1", MissionID: id, Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, Status: krdomain.KeyResultStatusOpen, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}}, nil
}

func (s *stubMissionService) UpdateKeyResult(ctx context.Context, req missionapp.UpdateKeyResultParams) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_update")
	return krdomain.KeyResult{ID: req.KeyResultID, MissionID: "mission-1", Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, Status: krdomain.KeyResultStatusOpen, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) DeleteKeyResult(ctx context.Context, req missionapp.DeleteKeyResultParams) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_drop")
	s.deleteKRReq = req
	return krdomain.KeyResult{ID: req.KeyResultID, MissionID: "mission-1", Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, Status: krdomain.KeyResultStatusDropped, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) ReopenKeyResult(ctx context.Context, req missionapp.ReopenKeyResultParams) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_reopen")
	s.reopenKRReq = req
	return krdomain.KeyResult{ID: req.KeyResultID, MissionID: "mission-1", Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, Status: krdomain.KeyResultStatusOpen, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) UpdateProgress(ctx context.Context, req missionapp.UpdateProgressParams) (krdomain.KeyResult, error) {
	s.capture(ctx, "kr_progress")
	s.progressReq = req
	return krdomain.KeyResult{ID: req.KeyResultID, MissionID: "mission-1", Title: "KR", MeasurementType: krdomain.MeasurementPercentage, Direction: krdomain.DirectionIncrease, TargetValue: 100, CurrentValue: req.Value, ProgressPercent: 50, Status: krdomain.KeyResultStatusInProgress, CreatedAt: missionTestTime, UpdatedAt: missionTestTime}, nil
}

func (s *stubMissionService) GetProgressHistory(ctx context.Context, id krdomain.KeyResultID) ([]krdomain.ProgressEntry, error) {
	s.capture(ctx, "kr_history")
	return []krdomain.ProgressEntry{{ID: "progress-1", KeyResultID: id, PreviousValue: 10, NewValue: 50, ProgressPercent: 50, Note: "halfway", CreatedAt: missionTestTime}}, nil
}

func (s *stubMissionService) ComputeMissionProgress(ctx context.Context, id missiondomain.MissionID) (missionapp.MissionProgress, error) {
	s.capture(ctx, "progress")
	return missionapp.MissionProgress{MissionID: id, ProgressPercent: 50, KeyResultCount: 2}, nil
}

type stubMembers struct{}

func (stubMembers) GetMember(context.Context, member.ID) (member.Record, error) {
	return member.Record{ID: "member-1", UserID: "user-1", ProjectID: "space-1", DisplayName: "Coordinator", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
}

// membersFunc adapts a function to the members lookup port. stubMembers always
// returns the same active record and never errors, so it cannot drive the
// inactive or load-error branches of actor(); tests that need those shapes use
// this instead.
type membersFunc func(context.Context, member.ID) (member.Record, error)

func (f membersFunc) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	return f(ctx, id)
}

func TestHandleCreateMissionCallsServiceWithSessionProjectAndCaller(t *testing.T) {
	svc := &stubMissionService{}
	result, err := NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"create","title":"Launch v1","description":"Ship it"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "create" {
		t.Fatalf("called=%q want create", svc.called)
	}
	if svc.createReq.ProjectID != "project-1" || svc.createReq.Title != "Launch v1" || svc.createReq.ID == "" {
		t.Fatalf("create req = %+v", svc.createReq)
	}
	if svc.seenCaller.UserID != "user-1" || svc.seenCaller.MemberID != "member-1" || svc.seenCaller.ProjectID != "space-1" {
		t.Fatalf("caller = %+v", svc.seenCaller)
	}
	if !strings.Contains(result.Text, `"action":"create"`) {
		t.Fatalf("text = %s", result.Text)
	}
}

// TestHandleRejectsMemberlessCaller locks the loud-failure half of the
// pre-registration affordance: the daemon resolves an unregistered session to a
// member-LESS session (no member, no error). Handle runs actor() for every action,
// so a blank actor member id must fail loudly before any mission service method
// runs. Members is non-nil here, so the failure is specifically the missing member
// id - not a missing member service.
func TestHandleRejectsMemberlessCaller(t *testing.T) {
	svc := &stubMissionService{}
	call := testCallContext(svc)
	call.ActorMemberID = ""
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","title":"Launch v1","description":"Ship it"}`))
	if err == nil || !strings.Contains(err.Error(), "mission: actor member id is required") {
		t.Fatalf("err=%v want mission actor member id required", err)
	}
	if svc.called != "" {
		t.Fatalf("mission service action %q ran despite member-less caller", svc.called)
	}
}

// actor() runs for every mission action before the service is touched. Beyond
// the member-less case above, it must reject a member it cannot load and a
// member that is no longer active. Both tests assert svc.called stays "" — the
// guard fires before dispatch reaches the mission service.

func TestHandleRejectsInactiveActor(t *testing.T) {
	svc := &stubMissionService{}
	call := testCallContext(svc)
	// Non-empty ID so the actor.ID=="" guard (handler.go:109) passes and the
	// inactive check (112) is what rejects.
	call.Members = membersFunc(func(_ context.Context, id member.ID) (member.Record, error) {
		return member.Record{ID: id, ProjectID: "space-1", LifecycleState: member.LifecycleRemoved}, nil
	})
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","title":"Launch v1","description":"Ship it"}`))
	if err == nil || !strings.Contains(err.Error(), "mission: actor member is not active") {
		t.Fatalf("err=%v want inactive actor", err)
	}
	if svc.called != "" {
		t.Fatalf("mission service action %q ran despite inactive actor", svc.called)
	}
}

func TestHandleRejectsActorLoadError(t *testing.T) {
	svc := &stubMissionService{}
	call := testCallContext(svc)
	call.Members = membersFunc(func(context.Context, member.ID) (member.Record, error) {
		return member.Record{}, member.ErrNotFound
	})
	_, err := NewHandler().Handle(context.Background(), call, json.RawMessage(`{"action":"create","title":"Launch v1","description":"Ship it"}`))
	if err == nil || !strings.Contains(err.Error(), "mission: load actor member") {
		t.Fatalf("err=%v want load actor member", err)
	}
	if !errors.Is(err, member.ErrNotFound) {
		t.Fatalf("error does not wrap member.ErrNotFound: %v", err)
	}
	if svc.called != "" {
		t.Fatalf("mission service action %q ran despite actor load error", svc.called)
	}
}

func TestHandleKRProgressCallsService(t *testing.T) {
	svc := &stubMissionService{}
	_, err := NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"kr_progress","key_result_id":"kr-1","value":50,"note":"halfway","expected_version":7}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "kr_progress" {
		t.Fatalf("called=%q want kr_progress", svc.called)
	}
	if svc.progressReq.KeyResultID != "kr-1" || svc.progressReq.Value != 50 || svc.progressReq.Note != "halfway" || svc.progressReq.ExpectedVersion != 7 {
		t.Fatalf("progress req = %+v", svc.progressReq)
	}
}

func TestHandleHistoryReturnsLifecycleNotes(t *testing.T) {
	svc := &stubMissionService{}
	result, err := NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"history","mission_id":"mission-1","limit":10,"offset":0}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "history" {
		t.Fatalf("called=%q want history", svc.called)
	}
	for _, want := range []string{`"action":"history"`, `"note":"ready"`, `"limit":10`, `"offset":0`} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("text missing %s: %s", want, result.Text)
		}
	}
}

func TestHandleActivateMapsToUpdateMissionStatus(t *testing.T) {
	svc := &stubMissionService{}
	result, err := NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"activate","mission_id":"mission-1","note":"ready"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if svc.called != "update" || svc.updateReq.MissionID != "mission-1" || svc.updateReq.Status == nil || *svc.updateReq.Status != missiondomain.MissionStatusActive || svc.updateReq.Note != "ready" {
		t.Fatalf("update req = %+v called=%q", svc.updateReq, svc.called)
	}
	if !strings.Contains(result.Text, `"note":"ready"`) {
		t.Fatalf("text = %s", result.Text)
	}
}

func TestHandleLifecycleActionsAcceptNotesAndKRListPagination(t *testing.T) {
	svc := &stubMissionService{}
	result, err := NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"complete","mission_id":"mission-1","note":"done"}`))
	if err != nil {
		t.Fatalf("handle complete: %v", err)
	}
	if svc.updateReq.Note != "done" {
		t.Fatalf("update note=%q want done", svc.updateReq.Note)
	}
	if !strings.Contains(result.Text, `"note":"done"`) {
		t.Fatalf("text = %s", result.Text)
	}

	result, err = NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"kr_drop","key_result_id":"kr-1","note":"out of scope"}`))
	if err != nil {
		t.Fatalf("handle kr_drop: %v", err)
	}
	if svc.deleteKRReq.Note != "out of scope" {
		t.Fatalf("drop note=%q want out of scope", svc.deleteKRReq.Note)
	}
	if !strings.Contains(result.Text, `"note":"out of scope"`) {
		t.Fatalf("text = %s", result.Text)
	}

	result, err = NewHandler().Handle(context.Background(), testCallContext(svc), json.RawMessage(`{"action":"kr_list","mission_id":"mission-1","limit":1,"offset":0}`))
	if err != nil {
		t.Fatalf("handle kr_list: %v", err)
	}
	if !strings.Contains(result.Text, `"limit":1`) || !strings.Contains(result.Text, `"offset":0`) {
		t.Fatalf("text = %s", result.Text)
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

func testCallContext(svc *stubMissionService) CallContext {
	return CallContext{
		Missions:      svc,
		KeyResults:    svc,
		Progress:      svc,
		Members:       stubMembers{},
		ProjectID:     "project-1",
		ActorMemberID: "member-1",
	}
}

func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"create","goal":"old"}`))
	if err == nil || !strings.Contains(err.Error(), `field "goal" is not valid for action "create"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list","project_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "project_id" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsFieldFromOtherAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"list","key_result_id":"kr-1"}`))
	if err == nil || !strings.Contains(err.Error(), `field "key_result_id" is not valid for action "list"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

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

func TestSchemaIncludesMissionAndKRActions(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(NewHandler().Schema(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "action" {
		t.Fatalf("required=%v want only action", required)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["key_result_id"]; !ok {
		t.Fatalf("schema missing key_result_id")
	}
	if _, ok := props["key_result_id"].(map[string]any)["anyOf"]; ok {
		t.Fatalf("key_result_id should not be nullable: %+v", props["key_result_id"])
	}
	action := props["action"].(map[string]any)
	enums := action["enum"].([]any)
	foundProgress := false
	for _, value := range enums {
		if value == "kr_progress" {
			foundProgress = true
		}
	}
	if !foundProgress {
		t.Fatalf("schema missing kr_progress action")
	}
}
