package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type Handler struct {
	service *missionapp.Service
}

func NewHandler(service *missionapp.Service) *Handler {
	if service == nil {
		panic("mission RPC handler requires mission service")
	}
	return &Handler{service: service}
}

func (h *Handler) Create(ctx context.Context, params CreateMissionParams) (CreateMissionResult, error) {
	projectID := strings.TrimSpace(params.ProjectID)
	if projectID == "" {
		return CreateMissionResult{}, invalidParams("projectId is required")
	}
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return CreateMissionResult{}, invalidParams("title is required")
	}
	startDate, err := parseOptionalDate(params.StartDate, "startDate")
	if err != nil {
		return CreateMissionResult{}, err
	}
	endDate, err := parseOptionalDate(params.EndDate, "endDate")
	if err != nil {
		return CreateMissionResult{}, err
	}
	mission, err := h.service.CreateMission(ctx, missionapp.CreateMissionParams{
		ID:          missiondomain.MissionID("mission-" + uuid.NewString()),
		ProjectID:   projectID,
		Title:       title,
		Description: params.Description,
		StartDate:   startDate,
		EndDate:     endDate,
	})
	if err != nil {
		return CreateMissionResult{}, internalError("create mission", err)
	}
	return CreateMissionResult{Mission: missionView(mission)}, nil
}

func (h *Handler) Get(ctx context.Context, params GetMissionParams) (GetMissionResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return GetMissionResult{}, invalidParams("missionId is required")
	}
	mission, err := h.service.GetMission(ctx, missiondomain.MissionID(missionID))
	if err != nil {
		return GetMissionResult{}, internalError("get mission", err)
	}
	return GetMissionResult{Mission: missionView(mission)}, nil
}

func (h *Handler) List(ctx context.Context, params ListMissionsParams) (ListMissionsResult, error) {
	if params.Limit < 0 {
		return ListMissionsResult{}, invalidParams("limit must be non-negative")
	}
	if params.Offset < 0 {
		return ListMissionsResult{}, invalidParams("offset must be non-negative")
	}
	projectID := strings.TrimSpace(params.ProjectID)
	if projectID == "" {
		return ListMissionsResult{}, invalidParams("projectId is required")
	}
	statuses := make([]missiondomain.MissionStatus, 0, len(params.Status))
	for _, raw := range params.Status {
		status := missiondomain.MissionStatus(strings.TrimSpace(raw))
		if status == "" {
			continue
		}
		statuses = append(statuses, status)
	}
	missions, err := h.service.ListMissions(ctx, projectID, missiondomain.MissionFilter{
		Statuses: statuses,
		Limit:    params.Limit,
		Offset:   params.Offset,
	})
	if err != nil {
		return ListMissionsResult{}, internalError("list missions", err)
	}
	out := make([]MissionView, 0, len(missions))
	for _, mission := range missions {
		out = append(out, missionView(mission))
	}
	return ListMissionsResult{Missions: out}, nil
}

func (h *Handler) Update(ctx context.Context, params UpdateMissionParams) (UpdateMissionResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return UpdateMissionResult{}, invalidParams("missionId is required")
	}
	var status *missiondomain.MissionStatus
	if params.Status != nil {
		s := missiondomain.MissionStatus(strings.TrimSpace(*params.Status))
		status = &s
	}
	startDate, err := parseOptionalDatePtr(params.StartDate, "startDate")
	if err != nil {
		return UpdateMissionResult{}, err
	}
	endDate, err := parseOptionalDatePtr(params.EndDate, "endDate")
	if err != nil {
		return UpdateMissionResult{}, err
	}
	mission, err := h.service.UpdateMission(ctx, missionapp.UpdateMissionParams{
		MissionID:   missiondomain.MissionID(missionID),
		Title:       params.Title,
		Description: params.Description,
		Status:      status,
		StartDate:   startDate,
		EndDate:     endDate,
		Note:        params.Note,
	})
	if err != nil {
		return UpdateMissionResult{}, serviceError("update mission", err)
	}
	return UpdateMissionResult{Mission: missionView(mission)}, nil
}

func (h *Handler) Delete(ctx context.Context, params DeleteMissionParams) (DeleteMissionResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return DeleteMissionResult{}, invalidParams("missionId is required")
	}
	mission, err := h.service.DeleteMission(ctx, missionapp.DeleteMissionParams{
		MissionID: missiondomain.MissionID(missionID),
		Note:      params.Note,
	})
	if err != nil {
		return DeleteMissionResult{}, internalError("delete mission", err)
	}
	return DeleteMissionResult{Mission: missionView(mission)}, nil
}

func (h *Handler) Purge(ctx context.Context, params PurgeMissionParams) (PurgeMissionResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return PurgeMissionResult{}, invalidParams("missionId is required")
	}
	mission, err := h.service.HardDeleteMission(ctx, missionapp.HardDeleteMissionParams{
		MissionID: missiondomain.MissionID(missionID),
		Note:      params.Note,
	})
	if err != nil {
		return PurgeMissionResult{}, serviceError("purge mission", err)
	}
	return PurgeMissionResult{Mission: missionView(mission)}, nil
}

func (h *Handler) CreateKeyResult(ctx context.Context, params CreateKeyResultParams) (CreateKeyResultResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return CreateKeyResultResult{}, invalidParams("missionId is required")
	}
	title := strings.TrimSpace(params.Title)
	if title == "" {
		return CreateKeyResultResult{}, invalidParams("title is required")
	}
	measurementType := strings.TrimSpace(params.MeasurementType)
	if measurementType == "" {
		return CreateKeyResultResult{}, invalidParams("measurementType is required")
	}
	kr, err := h.service.CreateKeyResult(ctx, missionapp.CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-" + uuid.NewString()),
		MissionID:       missiondomain.MissionID(missionID),
		Title:           title,
		Description:     params.Description,
		MeasurementType: krdomain.MeasurementType(measurementType),
		Direction:       krdomain.MeasurementDirection(strings.TrimSpace(params.Direction)),
		Unit:            params.Unit,
		Baseline:        params.Baseline,
		TargetValue:     params.TargetValue,
	})
	if err != nil {
		return CreateKeyResultResult{}, internalError("create key result", err)
	}
	return CreateKeyResultResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) GetKeyResult(ctx context.Context, params GetKeyResultParams) (GetKeyResultResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return GetKeyResultResult{}, invalidParams("keyResultId is required")
	}
	kr, err := h.service.GetKeyResult(ctx, krdomain.KeyResultID(keyResultID))
	if err != nil {
		return GetKeyResultResult{}, internalError("get key result", err)
	}
	return GetKeyResultResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) ListKeyResults(ctx context.Context, params ListKeyResultsParams) (ListKeyResultsResult, error) {
	if params.Limit < 0 {
		return ListKeyResultsResult{}, invalidParams("limit must be non-negative")
	}
	if params.Offset < 0 {
		return ListKeyResultsResult{}, invalidParams("offset must be non-negative")
	}
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return ListKeyResultsResult{}, invalidParams("missionId is required")
	}
	krs, err := h.service.ListKeyResults(ctx, missiondomain.MissionID(missionID))
	if err != nil {
		return ListKeyResultsResult{}, internalError("list key results", err)
	}
	krs = sliceKeyResultViews(krs, params.Limit, params.Offset)
	out := make([]KeyResultView, 0, len(krs))
	for _, kr := range krs {
		out = append(out, keyResultView(kr))
	}
	return ListKeyResultsResult{KeyResults: out}, nil
}

func (h *Handler) UpdateKeyResult(ctx context.Context, params UpdateKeyResultParams) (UpdateKeyResultResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return UpdateKeyResultResult{}, invalidParams("keyResultId is required")
	}
	appParams := missionapp.UpdateKeyResultParams{
		KeyResultID: krdomain.KeyResultID(keyResultID),
		Title:       params.Title,
		Description: params.Description,
		Unit:        params.Unit,
		Baseline:    params.Baseline,
		TargetValue: params.TargetValue,
	}
	if params.MeasurementType != nil {
		v := krdomain.MeasurementType(strings.TrimSpace(*params.MeasurementType))
		appParams.MeasurementType = &v
	}
	if params.Direction != nil {
		v := krdomain.MeasurementDirection(strings.TrimSpace(*params.Direction))
		appParams.Direction = &v
	}
	kr, err := h.service.UpdateKeyResult(ctx, appParams)
	if err != nil {
		return UpdateKeyResultResult{}, internalError("update key result", err)
	}
	return UpdateKeyResultResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) DeleteKeyResult(ctx context.Context, params DeleteKeyResultParams) (DeleteKeyResultResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return DeleteKeyResultResult{}, invalidParams("keyResultId is required")
	}
	kr, err := h.service.DeleteKeyResult(ctx, missionapp.DeleteKeyResultParams{
		KeyResultID: krdomain.KeyResultID(keyResultID),
		Note:        params.Note,
	})
	if err != nil {
		return DeleteKeyResultResult{}, internalError("delete key result", err)
	}
	return DeleteKeyResultResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) ReopenKeyResult(ctx context.Context, params ReopenKeyResultParams) (ReopenKeyResultResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return ReopenKeyResultResult{}, invalidParams("keyResultId is required")
	}
	kr, err := h.service.ReopenKeyResult(ctx, missionapp.ReopenKeyResultParams{
		KeyResultID: krdomain.KeyResultID(keyResultID),
		Note:        params.Note,
	})
	if err != nil {
		return ReopenKeyResultResult{}, internalError("reopen key result", err)
	}
	return ReopenKeyResultResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) UpdateProgress(ctx context.Context, params UpdateProgressParams) (UpdateProgressResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return UpdateProgressResult{}, invalidParams("keyResultId is required")
	}
	kr, err := h.service.UpdateProgress(ctx, missionapp.UpdateProgressParams{
		KeyResultID:     krdomain.KeyResultID(keyResultID),
		Value:           params.Value,
		Note:            params.Note,
		ExpectedVersion: params.ExpectedVersion,
	})
	if err != nil {
		return UpdateProgressResult{}, internalError("update progress", err)
	}
	return UpdateProgressResult{KeyResult: keyResultView(kr)}, nil
}

func (h *Handler) ProgressHistory(ctx context.Context, params ProgressHistoryParams) (ProgressHistoryResult, error) {
	keyResultID := strings.TrimSpace(params.KeyResultID)
	if keyResultID == "" {
		return ProgressHistoryResult{}, invalidParams("keyResultId is required")
	}
	entries, err := h.service.GetProgressHistory(ctx, krdomain.KeyResultID(keyResultID))
	if err != nil {
		return ProgressHistoryResult{}, internalError("get progress history", err)
	}
	out := make([]ProgressEntryView, 0, len(entries))
	for _, entry := range entries {
		out = append(out, ProgressEntryView{
			ID:              entry.ID,
			KeyResultID:     string(entry.KeyResultID),
			PreviousValue:   entry.PreviousValue,
			NewValue:        entry.NewValue,
			ProgressPercent: entry.ProgressPercent,
			UpdatedBy:       entry.UpdatedBy,
			Note:            entry.Note,
			CreatedAt:       entry.CreatedAt.Format(time.RFC3339),
		})
	}
	return ProgressHistoryResult{Entries: out}, nil
}

func (h *Handler) Progress(ctx context.Context, params MissionProgressParams) (MissionProgressResult, error) {
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return MissionProgressResult{}, invalidParams("missionId is required")
	}
	progress, err := h.service.ComputeMissionProgress(ctx, missiondomain.MissionID(missionID))
	if err != nil {
		return MissionProgressResult{}, internalError("get mission progress", err)
	}
	return missionProgressResult(progress), nil
}

func (h *Handler) LifecycleHistory(ctx context.Context, params LifecycleHistoryParams) (LifecycleHistoryResult, error) {
	if params.Limit < 0 {
		return LifecycleHistoryResult{}, invalidParams("limit must be non-negative")
	}
	if params.Offset < 0 {
		return LifecycleHistoryResult{}, invalidParams("offset must be non-negative")
	}
	missionID := strings.TrimSpace(params.MissionID)
	if missionID == "" {
		return LifecycleHistoryResult{}, invalidParams("missionId is required")
	}
	history, err := h.service.GetLifecycleHistory(ctx, missiondomain.MissionID(missionID), missionapp.LifecycleHistoryFilter{
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		return LifecycleHistoryResult{}, internalError("get mission lifecycle history", err)
	}
	return lifecycleHistoryResult(history), nil
}

func sliceKeyResultViews(keyResults []krdomain.KeyResult, limit int, offset int) []krdomain.KeyResult {
	if limit == 0 {
		limit = len(keyResults)
	}
	if offset >= len(keyResults) {
		return []krdomain.KeyResult{}
	}
	end := offset + limit
	if end > len(keyResults) {
		end = len(keyResults)
	}
	return keyResults[offset:end]
}

func lifecycleHistoryResult(history missionapp.LifecycleHistory) LifecycleHistoryResult {
	entries := make([]LifecycleHistoryEntryView, 0, len(history.Entries))
	for _, entry := range history.Entries {
		entries = append(entries, LifecycleHistoryEntryView{
			EventID:         entry.EventID,
			MissionID:       string(entry.MissionID),
			KeyResultID:     string(entry.KeyResultID),
			Type:            entry.Type,
			Action:          entry.Action,
			Status:          entry.Status,
			Note:            entry.Note,
			Actor:           entry.Actor,
			Origin:          entry.Origin,
			Message:         entry.Message,
			ProgressPercent: entry.ProgressPercent,
			Timestamp:       entry.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	return LifecycleHistoryResult{
		MissionID: string(history.MissionID),
		Entries:   entries,
		Count:     history.Count,
		Limit:     history.Limit,
		Offset:    history.Offset,
	}
}

func missionProgressResult(progress missionapp.MissionProgress) MissionProgressResult {
	statusCounts := make(map[string]int, len(progress.StatusCounts))
	for status, count := range progress.StatusCounts {
		statusCounts[string(status)] = count
	}
	blocking := make([]MissionProgressKRSummary, 0, len(progress.BlockingKeyResults))
	for _, keyResult := range progress.BlockingKeyResults {
		blocking = append(blocking, MissionProgressKRSummary{
			ID:              string(keyResult.ID),
			Title:           keyResult.Title,
			Status:          string(keyResult.Status),
			ProgressPercent: keyResult.ProgressPercent,
		})
	}
	return MissionProgressResult{
		MissionID:          string(progress.MissionID),
		ProgressPercent:    progress.ProgressPercent,
		KeyResultCount:     progress.KeyResultCount,
		StatusCounts:       statusCounts,
		BlockingKeyResults: blocking,
	}
}

func missionView(mission missiondomain.Mission) MissionView {
	return MissionView{
		ID:          string(mission.ID),
		ProjectID:   mission.ProjectID,
		Title:       mission.Title,
		Description: mission.Description,
		Status:      string(mission.Status),
		StartDate:   formatOptionalTime(mission.StartDate),
		EndDate:     formatOptionalTime(mission.EndDate),
		CreatedAt:   mission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   mission.UpdatedAt.Format(time.RFC3339),
		PausedAt:    formatOptionalTime(mission.PausedAt),
		CompletedAt: formatOptionalTime(mission.CompletedAt),
	}
}

func parseOptionalDate(raw string, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, invalidParams(field + " must use YYYY-MM-DD")
	}
	return &parsed, nil
}

func parseOptionalDatePtr(raw *string, field string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	return parseOptionalDate(*raw, field)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

func serviceError(action string, err error) error {
	var validation missionapp.ValidationError
	if errors.As(err, &validation) {
		return invalidParams(fmt.Sprintf("%s: %s", action, validation.Error()))
	}
	return internalError(action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}

func keyResultView(kr krdomain.KeyResult) KeyResultView {
	return KeyResultView{
		ID:                    string(kr.ID),
		MissionID:             string(kr.MissionID),
		Title:                 kr.Title,
		Description:           kr.Description,
		MeasurementType:       string(kr.MeasurementType),
		Direction:             string(kr.Direction),
		Unit:                  kr.Unit,
		Baseline:              kr.Baseline,
		TargetValue:           kr.TargetValue,
		CurrentValue:          kr.CurrentValue,
		ProgressPercent:       kr.ProgressPercent,
		Status:                string(kr.Status),
		ProjectID:             kr.ProjectID,
		OwnerProjectName:      kr.OwnerProjectName,
		LastMilestoneNotified: kr.LastMilestoneNotified,
		Version:               kr.Version,
	}
}
