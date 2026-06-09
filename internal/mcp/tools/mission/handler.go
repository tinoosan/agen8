package mission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

type Handler struct{}

func NewHandler() Handler {
	return Handler{}
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	ctx = contextWithSessionActor(ctx, call.ActorMemberID, call.ProjectID)
	actor, err := h.actor(ctx, call)
	if err != nil {
		return Result{}, err
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{UserID: actor.UserID, MemberID: actor.ID, ProjectID: types.ProjectID(actor.ProjectID)})

	switch input.Action {
	case "create":
		return h.create(ctx, call, input)
	case "get":
		return h.get(ctx, call, input)
	case "list":
		return h.list(ctx, call, input)
	case "update":
		return h.update(ctx, call, input, nil)
	case "activate":
		status := missiondomain.MissionStatusActive
		return h.update(ctx, call, input, &status)
	case "pause":
		status := missiondomain.MissionStatusPaused
		return h.update(ctx, call, input, &status)
	case "complete":
		status := missiondomain.MissionStatusCompleted
		return h.update(ctx, call, input, &status)
	case "archive":
		return h.archive(ctx, call, input)
	case "history":
		return h.history(ctx, call, input)
	case "kr_create":
		return h.krCreate(ctx, call, input)
	case "kr_get":
		return h.krGet(ctx, call, input)
	case "kr_list":
		return h.krList(ctx, call, input)
	case "kr_update":
		return h.krUpdate(ctx, call, input)
	case "kr_drop":
		return h.krDrop(ctx, call, input)
	case "kr_reopen":
		return h.krReopen(ctx, call, input)
	case "kr_progress":
		return h.krProgress(ctx, call, input)
	case "kr_history":
		return h.krHistory(ctx, call, input)
	case "progress":
		return h.progress(ctx, call, input)
	default:
		return Result{}, fmt.Errorf("mission: unsupported action %q", input.Action)
	}
}

func contextWithSessionActor(ctx context.Context, actorMemberID, projectID string) context.Context {
	actorMemberID = strings.TrimSpace(actorMemberID)
	projectID = strings.TrimSpace(projectID)
	if actorMemberID == "" && projectID == "" {
		return ctx
	}
	return caller.ContextWithCaller(ctx, caller.Caller{
		MemberID:  member.ID(actorMemberID),
		ProjectID: types.ProjectID(projectID),
	})
}

func (h Handler) actor(ctx context.Context, call CallContext) (member.Record, error) {
	if call.Members == nil {
		return member.Record{}, fmt.Errorf("mission: member service is not configured")
	}
	memberID := strings.TrimSpace(call.ActorMemberID)
	if memberID == "" {
		return member.Record{}, fmt.Errorf("mission: actor member id is required")
	}
	actor, err := call.Members.GetMember(ctx, member.ID(memberID))
	if err != nil {
		return member.Record{}, fmt.Errorf("mission: load actor member: %w", err)
	}
	if actor.ID == "" {
		return member.Record{}, fmt.Errorf("mission: actor member id is empty")
	}
	if strings.TrimSpace(actor.LifecycleState) != "" && strings.TrimSpace(actor.LifecycleState) != member.LifecycleActive {
		return member.Record{}, fmt.Errorf("mission: actor member is not active")
	}
	if strings.TrimSpace(actor.ProjectID) == "" {
		return member.Record{}, fmt.Errorf("mission: actor member project id is empty")
	}
	return actor, nil
}

func (h Handler) create(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	projectID := sessionProjectID(call, input)
	if projectID == "" {
		return Result{}, fmt.Errorf("mission: project_id is required")
	}
	title, err := requireString(input.Title, "title")
	if err != nil {
		return Result{}, err
	}
	startDate, err := parseOptionalDate(input.StartDate, "start_date")
	if err != nil {
		return Result{}, err
	}
	endDate, err := parseOptionalDate(input.EndDate, "end_date")
	if err != nil {
		return Result{}, err
	}
	mission, err := call.Missions.CreateMission(ctx, missionapp.CreateMissionParams{
		ID:          missiondomain.MissionID("mission-" + uuid.NewString()),
		ProjectID:   projectID,
		Title:       title,
		Description: input.Description,
		StartDate:   startDate,
		EndDate:     endDate,
	})
	if err != nil {
		return Result{}, err
	}
	return missionResult("create", mission)
}

func (h Handler) get(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	mission, err := call.Missions.GetMission(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return missionResult("get", mission)
}

func (h Handler) list(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	projectID := sessionProjectID(call, input)
	if projectID == "" {
		return Result{}, fmt.Errorf("mission: project_id is required")
	}
	filter := missiondomain.MissionFilter{Limit: input.Limit, Offset: input.Offset}
	if input.Status != "" {
		filter.Statuses = []missiondomain.MissionStatus{missiondomain.MissionStatus(input.Status)}
	}
	missions, err := call.Missions.ListMissions(ctx, projectID, filter)
	if err != nil {
		return Result{}, err
	}
	return missionListResult(missions, input)
}

func (h Handler) update(ctx context.Context, call CallContext, input requestInput, forcedStatus *missiondomain.MissionStatus) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	startDate, err := parseOptionalDatePtr(input.StartDate, "start_date")
	if err != nil {
		return Result{}, err
	}
	endDate, err := parseOptionalDatePtr(input.EndDate, "end_date")
	if err != nil {
		return Result{}, err
	}
	status := forcedStatus
	if status == nil && input.Status != "" {
		v := missiondomain.MissionStatus(input.Status)
		status = &v
	}
	params := missionapp.UpdateMissionParams{
		MissionID: id,
		Status:    status,
		StartDate: startDate,
		EndDate:   endDate,
		Note:      input.Note,
	}
	if input.Title != "" {
		params.Title = &input.Title
	}
	if input.Description != "" {
		params.Description = &input.Description
	}
	mission, err := call.Missions.UpdateMission(ctx, params)
	if err != nil {
		return Result{}, err
	}
	return missionResultWithNote(input.Action, mission, input.Note)
}

func (h Handler) archive(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	mission, err := call.Missions.DeleteMission(ctx, missionapp.DeleteMissionParams{MissionID: id, Note: input.Note})
	if err != nil {
		return Result{}, err
	}
	return missionResultWithNote("archive", mission, input.Note)
}

func (h Handler) history(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Missions == nil {
		return Result{}, fmt.Errorf("mission: mission service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	history, err := call.Missions.GetLifecycleHistory(ctx, id, missionapp.LifecycleHistoryFilter{
		Limit:  input.Limit,
		Offset: input.Offset,
	})
	if err != nil {
		return Result{}, err
	}
	return lifecycleHistoryResult(history)
}

func (h Handler) krCreate(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	missionID, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	title, err := requireString(input.Title, "title")
	if err != nil {
		return Result{}, err
	}
	if input.MeasurementType == "" {
		return Result{}, fmt.Errorf("mission: measurement_type is required")
	}
	if input.TargetValue == nil {
		return Result{}, fmt.Errorf("mission: target_value is required")
	}
	keyResult, err := call.KeyResults.CreateKeyResult(ctx, missionapp.CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-" + uuid.NewString()),
		MissionID:       missionID,
		Title:           title,
		Description:     input.Description,
		MeasurementType: krdomain.MeasurementType(input.MeasurementType),
		Direction:       krdomain.MeasurementDirection(input.Direction),
		Unit:            input.Unit,
		Baseline:        input.Baseline,
		TargetValue:     *input.TargetValue,
	})
	if err != nil {
		return Result{}, err
	}
	return keyResultResult("kr_create", keyResult)
}

func (h Handler) krGet(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	keyResult, err := call.KeyResults.GetKeyResult(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return keyResultResult("kr_get", keyResult)
}

func (h Handler) krList(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	keyResults, err := call.KeyResults.ListKeyResults(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return keyResultListResult(sliceKeyResults(keyResults, input), input)
}

func (h Handler) krUpdate(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	params := missionapp.UpdateKeyResultParams{KeyResultID: id, Unit: optionalString(input.Unit), Baseline: input.Baseline, TargetValue: input.TargetValue}
	if input.Title != "" {
		params.Title = &input.Title
	}
	if input.Description != "" {
		params.Description = &input.Description
	}
	if input.MeasurementType != "" {
		v := krdomain.MeasurementType(input.MeasurementType)
		params.MeasurementType = &v
	}
	if input.Direction != "" {
		v := krdomain.MeasurementDirection(input.Direction)
		params.Direction = &v
	}
	keyResult, err := call.KeyResults.UpdateKeyResult(ctx, params)
	if err != nil {
		return Result{}, err
	}
	return keyResultResult("kr_update", keyResult)
}

func (h Handler) krDrop(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	keyResult, err := call.KeyResults.DeleteKeyResult(ctx, missionapp.DeleteKeyResultParams{KeyResultID: id, Note: input.Note})
	if err != nil {
		return Result{}, err
	}
	return keyResultResultWithNote("kr_drop", keyResult, input.Note)
}

func (h Handler) krReopen(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.KeyResults == nil {
		return Result{}, fmt.Errorf("mission: key result service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	keyResult, err := call.KeyResults.ReopenKeyResult(ctx, missionapp.ReopenKeyResultParams{KeyResultID: id, Note: input.Note})
	if err != nil {
		return Result{}, err
	}
	return keyResultResultWithNote("kr_reopen", keyResult, input.Note)
}

func (h Handler) krProgress(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Progress == nil {
		return Result{}, fmt.Errorf("mission: progress service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	if input.Value == nil {
		return Result{}, fmt.Errorf("mission: value is required")
	}
	keyResult, err := call.Progress.UpdateProgress(ctx, missionapp.UpdateProgressParams{
		KeyResultID:     id,
		Value:           *input.Value,
		Note:            input.Note,
		ExpectedVersion: input.ExpectedVersion,
	})
	if err != nil {
		return Result{}, err
	}
	return keyResultResult("kr_progress", keyResult)
}

func (h Handler) krHistory(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Progress == nil {
		return Result{}, fmt.Errorf("mission: progress service is not configured")
	}
	id, err := requireKeyResultID(input.KeyResultID)
	if err != nil {
		return Result{}, err
	}
	entries, err := call.Progress.GetProgressHistory(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return historyResult(entries)
}

func (h Handler) progress(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if call.Progress == nil {
		return Result{}, fmt.Errorf("mission: progress service is not configured")
	}
	id, err := requireMissionID(input.MissionID)
	if err != nil {
		return Result{}, err
	}
	progress, err := call.Progress.ComputeMissionProgress(ctx, id)
	if err != nil {
		return Result{}, err
	}
	return progressResult(progress)
}

func sliceKeyResults(keyResults []krdomain.KeyResult, input requestInput) []krdomain.KeyResult {
	if input.Offset >= len(keyResults) {
		return []krdomain.KeyResult{}
	}
	limit := input.Limit
	if limit == 0 {
		limit = len(keyResults)
	}
	end := input.Offset + limit
	if end > len(keyResults) {
		end = len(keyResults)
	}
	return keyResults[input.Offset:end]
}

func sessionProjectID(call CallContext, input requestInput) string {
	if input.ProjectID != "" {
		return input.ProjectID
	}
	return strings.TrimSpace(call.ProjectID)
}

func requireMissionID(value string) (missiondomain.MissionID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("mission: mission_id is required")
	}
	return missiondomain.MissionID(value), nil
}

func requireKeyResultID(value string) (krdomain.KeyResultID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("mission: key_result_id is required")
	}
	return krdomain.KeyResultID(value), nil
}

func requireString(value string, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("mission: %s is required", field)
	}
	return value, nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func parseOptionalDate(raw string, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, fmt.Errorf("mission: %s must use YYYY-MM-DD", field)
	}
	return &parsed, nil
}

func parseOptionalDatePtr(raw string, field string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return parseOptionalDate(raw, field)
}
