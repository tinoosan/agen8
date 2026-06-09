package app

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/caller"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

type Service struct {
	missions        MissionRepository
	keyResults      KeyResultRepository
	progressEntries ProgressEntryRepository
	lifecycleEvents LifecycleEventRepository
	clock           missiondomain.Clock
	caller          caller.Resolver
	tasks           TaskLoader
	linkedTasks     LinkedTaskLoader
	events          EventPublisher
	logger          *slog.Logger
}

func NewService(
	missions MissionRepository,
	keyResults KeyResultRepository,
	progressEntries ProgressEntryRepository,
	lifecycleEvents LifecycleEventRepository,
	clock missiondomain.Clock,
	caller caller.Resolver,
	tasks TaskLoader,
	linkedTasks LinkedTaskLoader,
	events EventPublisher,
	logger *slog.Logger,
) (*Service, error) {
	switch {
	case missions == nil:
		return nil, fmt.Errorf("mission service: mission repository is required")
	case keyResults == nil:
		return nil, fmt.Errorf("mission service: key result repository is required")
	case progressEntries == nil:
		return nil, fmt.Errorf("mission service: progress entry repository is required")
	case lifecycleEvents == nil:
		return nil, fmt.Errorf("mission service: lifecycle event repository is required")
	case clock == nil:
		return nil, fmt.Errorf("mission service: clock is required")
	case caller == nil:
		return nil, fmt.Errorf("mission service: caller resolver is required")
	case tasks == nil:
		return nil, fmt.Errorf("mission service: task loader is required")
	case linkedTasks == nil:
		return nil, fmt.Errorf("mission service: linked task loader is required")
	case events == nil:
		return nil, fmt.Errorf("mission service: event publisher is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "mission")
	}
	return &Service{
		missions:        missions,
		keyResults:      keyResults,
		progressEntries: progressEntries,
		lifecycleEvents: lifecycleEvents,
		clock:           clock,
		caller:          caller,
		tasks:           tasks,
		linkedTasks:     linkedTasks,
		events:          events,
		logger:          logger,
	}, nil
}

type CreateMissionParams struct {
	ID          missiondomain.MissionID
	ProjectID   string
	Title       string
	Description string
	StartDate   *time.Time
	EndDate     *time.Time
}

type UpdateMissionParams struct {
	MissionID   missiondomain.MissionID
	Title       *string
	Description *string
	Status      *missiondomain.MissionStatus
	StartDate   *time.Time
	EndDate     *time.Time
	Note        string
}

type CreateKeyResultParams struct {
	ID              krdomain.KeyResultID
	MissionID       missiondomain.MissionID
	Title           string
	Description     string
	MeasurementType krdomain.MeasurementType
	Direction       krdomain.MeasurementDirection
	Unit            string
	Baseline        *float64
	TargetValue     float64
}

type UpdateKeyResultParams struct {
	KeyResultID     krdomain.KeyResultID
	Title           *string
	Description     *string
	MeasurementType *krdomain.MeasurementType
	Direction       *krdomain.MeasurementDirection
	Unit            *string
	Baseline        *float64
	TargetValue     *float64
}

type UpdateProgressParams struct {
	KeyResultID     krdomain.KeyResultID
	Value           float64
	Note            string
	ExpectedVersion int64
}

type DeleteMissionParams struct {
	MissionID missiondomain.MissionID
	Note      string
}

type HardDeleteMissionParams struct {
	MissionID missiondomain.MissionID
	Note      string
}

type DeleteKeyResultParams struct {
	KeyResultID krdomain.KeyResultID
	Note        string
}

type ReopenKeyResultParams struct {
	KeyResultID krdomain.KeyResultID
	Note        string
}

type MissionProgress struct {
	MissionID          missiondomain.MissionID
	ProgressPercent    int
	KeyResultCount     int
	StatusCounts       map[krdomain.KeyResultStatus]int
	BlockingKeyResults []MissionProgressKeyResult
}

type MissionProgressKeyResult struct {
	ID              krdomain.KeyResultID
	Title           string
	Status          krdomain.KeyResultStatus
	ProgressPercent int
}

type LifecycleHistoryFilter struct {
	Limit  int
	Offset int
	Types  []string
}

type LifecycleHistory struct {
	MissionID missiondomain.MissionID
	Entries   []LifecycleHistoryEntry
	Count     int
	Limit     int
	Offset    int
}

type LifecycleHistoryEntry struct {
	EventID         string
	MissionID       missiondomain.MissionID
	KeyResultID     krdomain.KeyResultID
	Type            string
	Action          string
	Status          string
	Note            string
	Actor           string
	Origin          string
	Message         string
	ProgressPercent string
	Timestamp       time.Time
}

func (s *Service) CreateMission(ctx context.Context, params CreateMissionParams) (missiondomain.Mission, error) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:          params.ID,
		ProjectID:   params.ProjectID,
		Title:       params.Title,
		Description: params.Description,
		StartDate:   params.StartDate,
		EndDate:     params.EndDate,
		Now:         s.clock.Now(),
	})
	if err != nil {
		return missiondomain.Mission{}, err
	}
	if err := s.missions.CreateMission(ctx, mission); err != nil {
		return missiondomain.Mission{}, err
	}
	s.logMissionTransition("create", mission)
	return mission, nil
}

func (s *Service) GetMission(ctx context.Context, missionID missiondomain.MissionID) (missiondomain.Mission, error) {
	return s.missions.GetMission(ctx, missionID)
}

func (s *Service) ListMissions(ctx context.Context, projectID string, filter missiondomain.MissionFilter) ([]missiondomain.Mission, error) {
	filter.ProjectID = projectID
	return s.missions.ListMissions(ctx, filter)
}

func (s *Service) UpdateMission(ctx context.Context, params UpdateMissionParams) (missiondomain.Mission, error) {
	mission, err := s.missions.GetMission(ctx, params.MissionID)
	if err != nil {
		return missiondomain.Mission{}, err
	}
	if params.Title != nil || params.Description != nil {
		title := mission.Title
		if params.Title != nil {
			title = *params.Title
		}
		description := mission.Description
		if params.Description != nil {
			description = *params.Description
		}
		mission, err = mission.UpdateDetails(title, description, s.clock.Now())
		if err != nil {
			return missiondomain.Mission{}, err
		}
	}
	if params.StartDate != nil || params.EndDate != nil {
		startDate := mission.StartDate
		if params.StartDate != nil {
			startDate = params.StartDate
		}
		endDate := mission.EndDate
		if params.EndDate != nil {
			endDate = params.EndDate
		}
		mission, err = mission.UpdateSchedule(startDate, endDate, s.clock.Now())
		if err != nil {
			return missiondomain.Mission{}, err
		}
	}
	previousStatus := mission.Status
	if params.Status != nil {
		mission, err = s.applyMissionStatus(ctx, mission, *params.Status)
		if err != nil {
			return missiondomain.Mission{}, err
		}
	}
	if err := s.missions.UpdateMission(ctx, mission); err != nil {
		return missiondomain.Mission{}, err
	}
	if params.Status != nil && mission.Status != previousStatus {
		eventKind, ok := missionEventKindForStatus(*params.Status)
		if ok {
			if err := s.publishMissionEventWithData(ctx, eventKind, mission, noteData(params.Note)); err != nil {
				return missiondomain.Mission{}, err
			}
		}
	} else if params.Status != nil {
		eventKind, ok := missionEventKindForStatus(*params.Status)
		if ok {
			if err := s.recordMissionLifecycleNote(ctx, eventKind, mission, params.Note); err != nil {
				return missiondomain.Mission{}, err
			}
		}
	}
	s.logMissionTransition("update", mission)
	return mission, nil
}

func missionEventKindForStatus(status missiondomain.MissionStatus) (MissionEventKind, bool) {
	switch status {
	case missiondomain.MissionStatusActive:
		return MissionEventActivated, true
	case missiondomain.MissionStatusPaused:
		return MissionEventPaused, true
	case missiondomain.MissionStatusCompleted:
		return MissionEventCompleted, true
	case missiondomain.MissionStatusArchived:
		return MissionEventArchived, true
	default:
		return "", false
	}
}

func (s *Service) DeleteMission(ctx context.Context, params DeleteMissionParams) (missiondomain.Mission, error) {
	mission, err := s.missions.GetMission(ctx, params.MissionID)
	if err != nil {
		return missiondomain.Mission{}, err
	}
	if mission.Status == missiondomain.MissionStatusArchived {
		if err := s.recordMissionLifecycleNote(ctx, MissionEventArchived, mission, params.Note); err != nil {
			return missiondomain.Mission{}, err
		}
		return mission, nil
	}
	mission, err = mission.Archive(s.clock.Now())
	if err != nil {
		return missiondomain.Mission{}, err
	}
	if err := s.missions.UpdateMission(ctx, mission); err != nil {
		return missiondomain.Mission{}, err
	}
	if err := s.publishMissionEventWithData(ctx, MissionEventArchived, mission, noteData(params.Note)); err != nil {
		return missiondomain.Mission{}, err
	}
	s.logMissionTransition("delete", mission)
	return mission, nil
}

// HardDeleteMission permanently removes a mission and everything beneath it
// (key results, progress entries, lifecycle events). Unlike DeleteMission,
// which only archives, this cannot be undone. We load the mission first so the
// caller gets a "not found" error for a bad id and so the purge event carries
// the real project id; then we cascade the delete in the repository and emit a
// purge event for live subscribers. The event is published after the delete
// succeeds so we never announce a removal that did not happen.
func (s *Service) HardDeleteMission(ctx context.Context, params HardDeleteMissionParams) (missiondomain.Mission, error) {
	mission, err := s.missions.GetMission(ctx, params.MissionID)
	if err != nil {
		return missiondomain.Mission{}, err
	}
	if err := s.missions.DeleteMission(ctx, mission.ID); err != nil {
		return missiondomain.Mission{}, err
	}
	if err := s.publishMissionEventWithData(ctx, MissionEventPurged, mission, noteData(params.Note)); err != nil {
		return missiondomain.Mission{}, err
	}
	s.logMissionTransition("purge", mission)
	return mission, nil
}

func (s *Service) CreateKeyResult(ctx context.Context, params CreateKeyResultParams) (krdomain.KeyResult, error) {
	mission, err := s.missions.GetMission(ctx, params.MissionID)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if mission.Status == missiondomain.MissionStatusArchived {
		return krdomain.KeyResult{}, fmt.Errorf("create key result: mission %s is archived", mission.ID)
	}
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              params.ID,
		MissionID:       params.MissionID,
		Title:           params.Title,
		Description:     params.Description,
		MeasurementType: params.MeasurementType,
		Direction:       params.Direction,
		Unit:            params.Unit,
		Baseline:        params.Baseline,
		TargetValue:     params.TargetValue,
		Now:             s.clock.Now(),
	})
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.keyResults.CreateKeyResult(ctx, keyResult); err != nil {
		return krdomain.KeyResult{}, err
	}
	var activated *missiondomain.Mission
	if mission.Status == missiondomain.MissionStatusCompleted {
		nextMission, err := mission.Activate(s.clock.Now())
		if err != nil {
			return krdomain.KeyResult{}, err
		}
		if err := s.missions.UpdateMission(ctx, nextMission); err != nil {
			return krdomain.KeyResult{}, err
		}
		activated = &nextMission
	}
	if err := s.publishKREvent(ctx, KREventCreated, keyResult); err != nil {
		return krdomain.KeyResult{}, err
	}
	if activated != nil {
		if err := s.publishMissionEvent(ctx, MissionEventActivated, *activated); err != nil {
			return krdomain.KeyResult{}, err
		}
		s.logMissionTransition("reactivate", *activated)
	}
	s.logKeyResultTransition("create", keyResult)
	return keyResult, nil
}

func (s *Service) GetKeyResult(ctx context.Context, keyResultID krdomain.KeyResultID) (krdomain.KeyResult, error) {
	return s.keyResults.GetKeyResult(ctx, keyResultID)
}

func (s *Service) ListKeyResults(ctx context.Context, missionID missiondomain.MissionID) ([]krdomain.KeyResult, error) {
	return s.keyResults.ListKeyResults(ctx, missionID)
}

func (s *Service) UpdateKeyResult(ctx context.Context, params UpdateKeyResultParams) (krdomain.KeyResult, error) {
	keyResult, err := s.keyResults.GetKeyResult(ctx, params.KeyResultID)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	keyResult, err = keyResult.Update(krdomain.UpdateInput{
		Title:           params.Title,
		Description:     params.Description,
		MeasurementType: params.MeasurementType,
		Direction:       params.Direction,
		Unit:            params.Unit,
		Baseline:        params.Baseline,
		TargetValue:     params.TargetValue,
	}, s.clock.Now())
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.keyResults.UpdateKeyResult(ctx, keyResult); err != nil {
		return krdomain.KeyResult{}, err
	}
	s.logKeyResultTransition("update", keyResult)
	return keyResult, nil
}

func (s *Service) DeleteKeyResult(ctx context.Context, params DeleteKeyResultParams) (krdomain.KeyResult, error) {
	keyResult, err := s.keyResults.GetKeyResult(ctx, params.KeyResultID)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	previousStatus := keyResult.Status
	keyResult, err = keyResult.Drop(params.Note, s.clock.Now())
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if previousStatus == krdomain.KeyResultStatusDropped {
		if err := s.recordKRLifecycleNote(ctx, KREventDropped, keyResult, params.Note); err != nil {
			return krdomain.KeyResult{}, err
		}
		return keyResult, nil
	}
	if err := s.keyResults.UpdateKeyResult(ctx, keyResult); err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.recomputeMissionAfterKeyResultDrop(ctx, keyResult.MissionID); err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.publishKREventWithData(ctx, KREventDropped, keyResult, noteData(params.Note)); err != nil {
		return krdomain.KeyResult{}, err
	}
	s.logKeyResultTransition("delete", keyResult)
	return keyResult, nil
}

func (s *Service) ReopenKeyResult(ctx context.Context, params ReopenKeyResultParams) (krdomain.KeyResult, error) {
	keyResult, err := s.keyResults.GetKeyResult(ctx, params.KeyResultID)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	previousStatus := keyResult.Status
	next, err := keyResult.Reopen(params.Note, s.clock.Now())
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if previousStatus == next.Status {
		if err := s.recordKRLifecycleNote(ctx, KREventReopened, next, params.Note); err != nil {
			return krdomain.KeyResult{}, err
		}
		return next, nil
	}
	if err := s.keyResults.UpdateKeyResult(ctx, next); err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.reactivateMissionForRenewedScope(ctx, next.MissionID); err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.publishKREventWithData(ctx, KREventReopened, next, noteData(params.Note)); err != nil {
		return krdomain.KeyResult{}, err
	}
	s.logKeyResultTransition("reopen", next)
	return next, nil
}

func (s *Service) UpdateProgress(ctx context.Context, params UpdateProgressParams) (krdomain.KeyResult, error) {
	keyResultID := krdomain.KeyResultID(strings.TrimSpace(string(params.KeyResultID)))
	if keyResultID == "" {
		return krdomain.KeyResult{}, fmt.Errorf("mission service: key result id is required")
	}
	keyResult, err := s.keyResults.GetKeyResult(ctx, keyResultID)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if err := s.validateKeyResultProgressMissionState(ctx, keyResult.MissionID); err != nil {
		return krdomain.KeyResult{}, err
	}
	next, entry, err := keyResult.UpdateProgress(
		params.Value,
		params.Note,
		params.ExpectedVersion,
		"progress-"+uuid.NewString(),
		"",
		s.clock.Now(),
	)
	if err != nil {
		return krdomain.KeyResult{}, err
	}
	if next.Status == krdomain.KeyResultStatusCompleted {
		if err := s.validateLinkedTasksComplete(ctx, keyResult.ID); err != nil {
			return krdomain.KeyResult{}, err
		}
	}
	milestones := crossedMilestones(keyResult.LastMilestoneNotified, next.ProgressPercent)
	if len(milestones) > 0 {
		next.LastMilestoneNotified = milestones[len(milestones)-1]
	}
	if err := s.progressEntries.AppendProgressEntry(ctx, entry); err != nil {
		return krdomain.KeyResult{}, fmt.Errorf("append progress entry: %w", err)
	}
	if err := s.keyResults.UpdateKeyResult(ctx, next); err != nil {
		return krdomain.KeyResult{}, fmt.Errorf("update key result: %w", err)
	}
	if err := s.publishKREvent(ctx, KREventProgressUpdated, next); err != nil {
		return krdomain.KeyResult{}, err
	}
	for _, milestone := range milestones {
		if err := s.publishKRMilestoneEvent(ctx, next, milestone); err != nil {
			return krdomain.KeyResult{}, err
		}
	}
	if next.Status == krdomain.KeyResultStatusCompleted {
		if err := s.publishKREvent(ctx, KREventCompleted, next); err != nil {
			return krdomain.KeyResult{}, err
		}
		if err := s.completeMissionIfAllKeyResultsComplete(ctx, next.MissionID); err != nil {
			return krdomain.KeyResult{}, err
		}
	}
	s.logKeyResultTransition("progress", next)
	return next, nil
}

func (s *Service) ComputeMissionProgress(ctx context.Context, missionID missiondomain.MissionID) (MissionProgress, error) {
	missionID = missiondomain.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return MissionProgress{}, fmt.Errorf("mission service: mission id is required")
	}
	keyResults, err := s.keyResults.ListKeyResults(ctx, missionID)
	if err != nil {
		return MissionProgress{}, err
	}
	var total int
	var count int
	statusCounts := map[krdomain.KeyResultStatus]int{}
	blocking := make([]MissionProgressKeyResult, 0)
	for _, keyResult := range keyResults {
		statusCounts[keyResult.Status]++
		if keyResult.Status == krdomain.KeyResultStatusDropped {
			continue
		}
		total += keyResult.ProgressPercent
		count++
		if keyResult.Status != krdomain.KeyResultStatusCompleted {
			blocking = append(blocking, MissionProgressKeyResult{
				ID:              keyResult.ID,
				Title:           keyResult.Title,
				Status:          keyResult.Status,
				ProgressPercent: keyResult.ProgressPercent,
			})
		}
	}
	if count == 0 {
		return MissionProgress{MissionID: missionID, StatusCounts: statusCounts}, nil
	}
	return MissionProgress{
		MissionID:          missionID,
		ProgressPercent:    int(math.Round(float64(total) / float64(count))),
		KeyResultCount:     count,
		StatusCounts:       statusCounts,
		BlockingKeyResults: blocking,
	}, nil
}

func (s *Service) GetProgressHistory(ctx context.Context, keyResultID krdomain.KeyResultID) ([]krdomain.ProgressEntry, error) {
	return s.progressEntries.ListProgressEntries(ctx, keyResultID)
}

func crossedMilestones(lastNotified int, progress int) []int {
	thresholds := []int{25, 50, 75, 100}
	out := make([]int, 0, len(thresholds))
	for _, threshold := range thresholds {
		if threshold > lastNotified && threshold <= progress {
			out = append(out, threshold)
		}
	}
	return out
}

func (s *Service) applyMissionStatus(ctx context.Context, mission missiondomain.Mission, status missiondomain.MissionStatus) (missiondomain.Mission, error) {
	switch status {
	case missiondomain.MissionStatusActive:
		if mission.Status == missiondomain.MissionStatusActive {
			return mission, nil
		}
		if err := s.validateMissionActivation(ctx, mission.ID); err != nil {
			return missiondomain.Mission{}, err
		}
		return mission.Activate(s.clock.Now())
	case missiondomain.MissionStatusCompleted:
		if mission.Status == missiondomain.MissionStatusCompleted {
			return mission, nil
		}
		if err := s.validateMissionCompletion(ctx, mission.ID); err != nil {
			return missiondomain.Mission{}, err
		}
		return mission.Complete(s.clock.Now())
	case missiondomain.MissionStatusPaused:
		if mission.Status == missiondomain.MissionStatusPaused {
			return mission, nil
		}
		return mission.Pause(s.clock.Now())
	case missiondomain.MissionStatusArchived:
		if mission.Status == missiondomain.MissionStatusArchived {
			return mission, nil
		}
		return mission.Archive(s.clock.Now())
	case missiondomain.MissionStatusDraft:
		return missiondomain.Mission{}, validationError("mission service: draft is not a valid update target")
	default:
		return missiondomain.Mission{}, validationError("mission service: unsupported mission status %q", status)
	}
}

func (s *Service) validateMissionActivation(ctx context.Context, missionID missiondomain.MissionID) error {
	keyResults, err := s.keyResults.ListKeyResults(ctx, missionID)
	if err != nil {
		return err
	}
	liveCount := 0
	for _, keyResult := range keyResults {
		if keyResult.Status == krdomain.KeyResultStatusDropped {
			continue
		}
		liveCount++
	}
	if liveCount == 0 {
		return validationError("Add at least one key result before activating this mission.")
	}
	return nil
}

func (s *Service) validateMissionCompletion(ctx context.Context, missionID missiondomain.MissionID) error {
	keyResults, err := s.keyResults.ListKeyResults(ctx, missionID)
	if err != nil {
		return err
	}
	liveCount := 0
	for _, keyResult := range keyResults {
		if keyResult.Status == krdomain.KeyResultStatusDropped {
			continue
		}
		liveCount++
		if keyResult.Status != krdomain.KeyResultStatusCompleted {
			return validationError("Every active key result must be completed before completing this mission.")
		}
	}
	if liveCount == 0 {
		return validationError("Add at least one completed key result before completing this mission.")
	}
	return nil
}

func (s *Service) recomputeMissionAfterKeyResultDrop(ctx context.Context, missionID missiondomain.MissionID) error {
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if mission.Status == missiondomain.MissionStatusArchived {
		return nil
	}
	keyResults, err := s.keyResults.ListKeyResults(ctx, missionID)
	if err != nil {
		return err
	}
	liveCount := 0
	allCompleted := true
	for _, keyResult := range keyResults {
		if keyResult.Status == krdomain.KeyResultStatusDropped {
			continue
		}
		liveCount++
		if keyResult.Status != krdomain.KeyResultStatusCompleted {
			allCompleted = false
		}
	}
	var next missiondomain.Mission
	if liveCount == 0 {
		if mission.Status == missiondomain.MissionStatusPaused {
			return nil
		}
		next, err = mission.Pause(s.clock.Now())
	} else if allCompleted {
		if mission.Status == missiondomain.MissionStatusCompleted {
			return nil
		}
		next, err = mission.Complete(s.clock.Now())
	} else {
		if mission.Status == missiondomain.MissionStatusActive {
			return nil
		}
		next, err = mission.Activate(s.clock.Now())
	}
	if err != nil {
		return err
	}
	if err := s.missions.UpdateMission(ctx, next); err != nil {
		return err
	}
	if eventKind, ok := missionEventKindForStatus(next.Status); ok {
		if err := s.publishMissionEvent(ctx, eventKind, next); err != nil {
			return err
		}
	}
	s.logMissionTransition("recompute_after_kr_drop", next)
	return nil
}

func (s *Service) reactivateMissionForRenewedScope(ctx context.Context, missionID missiondomain.MissionID) error {
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if mission.Status != missiondomain.MissionStatusCompleted {
		return nil
	}
	next, err := mission.Activate(s.clock.Now())
	if err != nil {
		return err
	}
	if err := s.missions.UpdateMission(ctx, next); err != nil {
		return err
	}
	if err := s.publishMissionEvent(ctx, MissionEventActivated, next); err != nil {
		return err
	}
	s.logMissionTransition("reactivate", next)
	return nil
}

func (s *Service) validateKeyResultProgressMissionState(ctx context.Context, missionID missiondomain.MissionID) error {
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if mission.Status != missiondomain.MissionStatusActive {
		return validationError("Key result progress can be updated only after its mission is active.")
	}
	return nil
}

func (s *Service) completeMissionIfAllKeyResultsComplete(ctx context.Context, missionID missiondomain.MissionID) error {
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return err
	}
	if mission.Status == missiondomain.MissionStatusArchived || mission.Status == missiondomain.MissionStatusCompleted {
		return nil
	}
	if err := s.validateMissionCompletion(ctx, missionID); err != nil {
		return nil
	}
	next, err := mission.Complete(s.clock.Now())
	if err != nil {
		return err
	}
	if err := s.missions.UpdateMission(ctx, next); err != nil {
		return err
	}
	if err := s.publishMissionEvent(ctx, MissionEventCompleted, next); err != nil {
		return err
	}
	s.logMissionTransition("auto_complete", next)
	return nil
}

func (s *Service) validateLinkedTasksComplete(ctx context.Context, keyResultID krdomain.KeyResultID) error {
	taskIDs, err := s.linkedTasks.ListTaskIDsForKeyResult(ctx, keyResultID)
	if err != nil {
		return fmt.Errorf("list linked tasks: %w", err)
	}
	for _, taskID := range taskIDs {
		taskID = taskdomain.TaskID(strings.TrimSpace(string(taskID)))
		if taskID == "" {
			return fmt.Errorf("mission service: linked task id is required")
		}
		task, err := s.tasks.Get(ctx, taskID)
		if err != nil {
			return fmt.Errorf("load linked task %s: %w", taskID, err)
		}
		if !task.Status.IsTerminal() {
			return fmt.Errorf("mission service: linked task %s is not terminal", taskID)
		}
	}
	return nil
}

func (s *Service) logMissionTransition(action string, mission missiondomain.Mission) {
	s.logger.Info("mission transition",
		"action", action,
		"mission_id", string(mission.ID),
		"project_id", mission.ProjectID,
		"status", string(mission.Status),
	)
}

func (s *Service) logKeyResultTransition(action string, keyResult krdomain.KeyResult) {
	s.logger.Info("key result transition",
		"action", action,
		"key_result_id", string(keyResult.ID),
		"mission_id", string(keyResult.MissionID),
		"status", string(keyResult.Status),
		"measurement_type", string(keyResult.MeasurementType),
		"progress_percent", keyResult.ProgressPercent,
	)
}
