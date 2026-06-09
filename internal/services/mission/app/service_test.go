package app

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

var serviceTestNow = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

type fakeMissionRepository struct {
	missions  map[string]missiondomain.Mission
	updateErr error
	deleteErr error
	deleted   []string
}

func newFakeMissionRepository(missions ...missiondomain.Mission) *fakeMissionRepository {
	repo := &fakeMissionRepository{missions: map[string]missiondomain.Mission{}}
	for _, mission := range missions {
		repo.missions[string(mission.ID)] = mission
	}
	return repo
}

func (r *fakeMissionRepository) GetMission(_ context.Context, missionID missiondomain.MissionID) (missiondomain.Mission, error) {
	mission, ok := r.missions[string(missionID)]
	if !ok {
		return missiondomain.Mission{}, fmt.Errorf("mission %s not found", missionID)
	}
	return mission, nil
}

func (r *fakeMissionRepository) ListMissions(_ context.Context, filter missiondomain.MissionFilter) ([]missiondomain.Mission, error) {
	out := make([]missiondomain.Mission, 0, len(r.missions))
	for _, mission := range r.missions {
		if filter.ProjectID != "" && mission.ProjectID != filter.ProjectID {
			continue
		}
		out = append(out, mission)
	}
	return out, nil
}

func (r *fakeMissionRepository) CreateMission(_ context.Context, mission missiondomain.Mission) error {
	r.missions[string(mission.ID)] = mission
	return nil
}

func (r *fakeMissionRepository) UpdateMission(_ context.Context, mission missiondomain.Mission) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if _, ok := r.missions[string(mission.ID)]; !ok {
		return fmt.Errorf("mission %s not found", mission.ID)
	}
	r.missions[string(mission.ID)] = mission
	return nil
}

func (r *fakeMissionRepository) DeleteMission(_ context.Context, missionID missiondomain.MissionID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.missions, string(missionID))
	r.deleted = append(r.deleted, string(missionID))
	return nil
}

type fakeKeyResultRepository struct {
	keyResults map[string]krdomain.KeyResult
	createErr  error
	updateErr  error
}

func newFakeKeyResultRepository(keyResults ...krdomain.KeyResult) *fakeKeyResultRepository {
	repo := &fakeKeyResultRepository{keyResults: map[string]krdomain.KeyResult{}}
	for _, keyResult := range keyResults {
		repo.keyResults[string(keyResult.ID)] = keyResult
	}
	return repo
}

func (r *fakeKeyResultRepository) GetKeyResult(_ context.Context, keyResultID krdomain.KeyResultID) (krdomain.KeyResult, error) {
	keyResult, ok := r.keyResults[string(keyResultID)]
	if !ok {
		return krdomain.KeyResult{}, fmt.Errorf("key result %s not found", keyResultID)
	}
	return keyResult, nil
}

func (r *fakeKeyResultRepository) ListKeyResults(_ context.Context, missionID missiondomain.MissionID) ([]krdomain.KeyResult, error) {
	out := make([]krdomain.KeyResult, 0, len(r.keyResults))
	for _, keyResult := range r.keyResults {
		if keyResult.MissionID == missionID {
			out = append(out, keyResult)
		}
	}
	return out, nil
}

func (r *fakeKeyResultRepository) CreateKeyResult(_ context.Context, keyResult krdomain.KeyResult) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.keyResults[string(keyResult.ID)] = keyResult
	return nil
}

func (r *fakeKeyResultRepository) UpdateKeyResult(_ context.Context, keyResult krdomain.KeyResult) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if _, ok := r.keyResults[string(keyResult.ID)]; !ok {
		return fmt.Errorf("key result %s not found", keyResult.ID)
	}
	r.keyResults[string(keyResult.ID)] = keyResult
	return nil
}

type fakeProgressEntryRepository struct {
	entries []krdomain.ProgressEntry
	err     error
}

func (r *fakeProgressEntryRepository) AppendProgressEntry(_ context.Context, entry krdomain.ProgressEntry) error {
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, entry)
	return nil
}

func (r *fakeProgressEntryRepository) ListProgressEntries(_ context.Context, keyResultID krdomain.KeyResultID) ([]krdomain.ProgressEntry, error) {
	out := make([]krdomain.ProgressEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry.KeyResultID == keyResultID {
			out = append(out, entry)
		}
	}
	return out, nil
}

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

type fakeCallerResolver struct{}

func (fakeCallerResolver) ResolveCaller(context.Context) (caller.Caller, error) {
	return caller.Caller{UserID: "user-1"}, nil
}

type fakeTaskLoader struct{}

func (fakeTaskLoader) Get(context.Context, taskdomain.TaskID) (taskdomain.Task, error) {
	return taskdomain.Task{}, fmt.Errorf("task not found")
}

type fakeTaskLoaderWithTasks struct {
	tasks map[string]taskdomain.Task
}

func (l fakeTaskLoaderWithTasks) Get(_ context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error) {
	task, ok := l.tasks[string(taskID)]
	if !ok {
		return taskdomain.Task{}, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

type fakeLinkedTaskLoader struct {
	taskIDs []taskdomain.TaskID
	calls   int
	err     error
}

func (l *fakeLinkedTaskLoader) ListTaskIDsForKeyResult(_ context.Context, _ krdomain.KeyResultID) ([]taskdomain.TaskID, error) {
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return append([]taskdomain.TaskID(nil), l.taskIDs...), nil
}

type fakeEventPublisher struct {
	events []types.EventRecord
	err    error
}

func (p *fakeEventPublisher) Append(_ context.Context, event types.EventRecord) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}

type fakeLifecycleEventRepository struct {
	events []types.EventRecord
	err    error
}

func (r *fakeLifecycleEventRepository) AppendLifecycleEvent(_ context.Context, event types.EventRecord) error {
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, event)
	return nil
}

func (r *fakeLifecycleEventRepository) ListLifecycleEvents(_ context.Context, missionID missiondomain.MissionID, filter LifecycleHistoryFilter) ([]types.EventRecord, int, error) {
	if r.err != nil {
		return nil, 0, r.err
	}
	out := make([]types.EventRecord, 0, len(r.events))
	typeSet := map[string]struct{}{}
	for _, eventType := range filter.Types {
		if eventType != "" {
			typeSet[eventType] = struct{}{}
		}
	}
	for _, event := range r.events {
		if event.RunID != types.RunID(missionID) && event.Data["missionId"] != string(missionID) {
			continue
		}
		if len(typeSet) > 0 {
			if _, ok := typeSet[event.Type]; !ok {
				continue
			}
		}
		out = append(out, event)
	}
	count := len(out)
	if filter.Offset > 0 {
		if filter.Offset >= len(out) {
			return []types.EventRecord{}, count, nil
		}
		out = out[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(out) {
		out = out[:filter.Limit]
	}
	return out, count, nil
}

func hasEventType(events []types.EventRecord, kind MissionEventKind) bool {
	for _, event := range events {
		if event.Type == string(kind) {
			return true
		}
	}
	return false
}

func eventMilestones(events []types.EventRecord) []string {
	out := make([]string, 0)
	for _, event := range events {
		if event.Type == string(KREventMilestone) {
			out = append(out, event.Data["milestone"])
		}
	}
	return out
}

func newServiceForTest(t *testing.T, missions *fakeMissionRepository, keyResults *fakeKeyResultRepository) *Service {
	t.Helper()
	svc, err := NewService(
		missions,
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestMissionEventSpecUsesKind(t *testing.T) {
	got, err := missionEventSpec(MissionEventActivated)
	if err != nil {
		t.Fatalf("missionEventSpec: %v", err)
	}
	if got.EventType != string(MissionEventActivated) {
		t.Fatalf("eventType=%q want %q", got.EventType, MissionEventActivated)
	}
}

func TestCreateMissionLogsTransition(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil)).With("service", "mission")
	svc, err := NewService(
		newFakeMissionRepository(),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		logger,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.CreateMission(context.Background(), CreateMissionParams{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
	})
	if err != nil {
		t.Fatalf("CreateMission: %v", err)
	}

	out := logs.String()
	for _, want := range []string{
		`msg="mission transition"`,
		"service=mission",
		"action=create",
		"mission_id=mission-1",
		"project_id=project-1",
		"status=draft",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestUpdateMissionUpdatesDetails(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	repo := newFakeMissionRepository(mission)
	svc := newServiceForTest(t, repo, newFakeKeyResultRepository())
	title := "Updated launch"
	description := "Updated scope"

	got, err := svc.UpdateMission(context.Background(), UpdateMissionParams{
		MissionID:   mission.ID,
		Title:       &title,
		Description: &description,
	})
	if err != nil {
		t.Fatalf("UpdateMission: %v", err)
	}
	if got.Title != title {
		t.Fatalf("Title=%q want %q", got.Title, title)
	}
	if got.Description != description {
		t.Fatalf("Description=%q want %q", got.Description, description)
	}
	if !got.UpdatedAt.Equal(serviceTestNow) {
		t.Fatalf("UpdatedAt=%v want %v", got.UpdatedAt, serviceTestNow)
	}
	stored, err := repo.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Title != title {
		t.Fatalf("stored Title=%q want %q", stored.Title, title)
	}
}

func TestDeleteMissionArchivesMission(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	repo := newFakeMissionRepository(mission)
	svc := newServiceForTest(t, repo, newFakeKeyResultRepository())

	got, err := svc.DeleteMission(context.Background(), DeleteMissionParams{MissionID: mission.ID})
	if err != nil {
		t.Fatalf("DeleteMission: %v", err)
	}
	if got.Status != missiondomain.MissionStatusArchived {
		t.Fatalf("Status=%q want %q", got.Status, missiondomain.MissionStatusArchived)
	}
	stored, err := repo.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusArchived {
		t.Fatalf("stored Status=%q want %q", stored.Status, missiondomain.MissionStatusArchived)
	}
}

func TestDeleteMissionPublishesArchivedEventAfterPersistence(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.DeleteMission(context.Background(), DeleteMissionParams{MissionID: mission.ID}); err != nil {
		t.Fatalf("DeleteMission: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	if events.events[0].Type != string(MissionEventArchived) {
		t.Fatalf("event Type=%q want %q", events.events[0].Type, MissionEventArchived)
	}
}

func TestHardDeleteMissionRemovesMissionAndPublishesPurgedEvent(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Launch",
		Now:       serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	repo := newFakeMissionRepository(mission)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		repo,
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.HardDeleteMission(context.Background(), HardDeleteMissionParams{MissionID: mission.ID})
	if err != nil {
		t.Fatalf("HardDeleteMission: %v", err)
	}
	if got.ID != mission.ID {
		t.Fatalf("returned mission ID=%q want %q", got.ID, mission.ID)
	}
	if _, err := repo.GetMission(context.Background(), mission.ID); err == nil {
		t.Fatalf("mission still present after hard delete")
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != string(mission.ID) {
		t.Fatalf("repo.deleted=%v want [%s]", repo.deleted, mission.ID)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	if events.events[0].Type != string(MissionEventPurged) {
		t.Fatalf("event Type=%q want %q", events.events[0].Type, MissionEventPurged)
	}
}

func TestHardDeleteMissionReturnsErrorForMissingMission(t *testing.T) {
	repo := newFakeMissionRepository()
	events := &fakeEventPublisher{}
	svc, err := NewService(
		repo,
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.HardDeleteMission(context.Background(), HardDeleteMissionParams{MissionID: missiondomain.MissionID("missing")}); err == nil {
		t.Fatal("expected error for missing mission, got nil")
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("repo.deleted=%v want empty (no delete attempted)", repo.deleted)
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0 (no event for failed purge)", len(events.events))
	}
}

func TestCreateKeyResultPublishesCreatedEventAfterPersistence(t *testing.T) {
	keyResults := newFakeKeyResultRepository()
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.CreateKeyResult(context.Background(), CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
	})
	if err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if got.ID != "kr-1" {
		t.Fatalf("ID=%q want kr-1", got.ID)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	event := events.events[0]
	if event.Type != string(KREventCreated) {
		t.Fatalf("event Type=%q want %q", event.Type, KREventCreated)
	}
	if event.RunID != types.RunID(got.MissionID) {
		t.Fatalf("event RunID=%q want %q", event.RunID, got.MissionID)
	}
	for key, want := range map[string]string{
		"missionId":       string(got.MissionID),
		"keyResultId":     string(got.ID),
		"status":          string(got.Status),
		"measurementType": string(got.MeasurementType),
		"progressPercent": "0",
		// Regression guard: KR events must carry projectId (resolved from the
		// mission) or the SSE fan-out drops them — ef824e74 removed this.
		"projectId": "project-1",
	} {
		if event.Data[key] != want {
			t.Fatalf("event Data[%q]=%q want %q", key, event.Data[key], want)
		}
	}
	if _, err := keyResults.GetKeyResult(context.Background(), got.ID); err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
}

func TestCreateKeyResultRequiresExistingMission(t *testing.T) {
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.CreateKeyResult(context.Background(), CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("missing"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
	})
	if err == nil {
		t.Fatal("CreateKeyResult error is nil")
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestCreateKeyResultReactivatesCompletedMission(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusCompleted)
	missions := newFakeMissionRepository(mission)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.CreateKeyResult(context.Background(), CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
	}); err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusActive {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusActive)
	}
	if !hasEventType(events.events, KREventCreated) {
		t.Fatalf("events missing %q: %+v", KREventCreated, events.events)
	}
	if !hasEventType(events.events, MissionEventActivated) {
		t.Fatalf("events missing %q: %+v", MissionEventActivated, events.events)
	}
}

func TestCreateKeyResultDoesNotPublishCreatedEventWhenPersistenceFails(t *testing.T) {
	keyResults := newFakeKeyResultRepository()
	keyResults.createErr = fmt.Errorf("write failed")
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.CreateKeyResult(context.Background(), CreateKeyResultParams{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
	})
	if err == nil {
		t.Fatal("CreateKeyResult error is nil")
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestUpdateKeyResultPersistsUpdatedAggregate(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	repo := newFakeKeyResultRepository(keyResult)
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), repo)
	title := "Updated KR"
	target := 200.0

	got, err := svc.UpdateKeyResult(context.Background(), UpdateKeyResultParams{
		KeyResultID: keyResult.ID,
		Title:       &title,
		TargetValue: &target,
	})
	if err != nil {
		t.Fatalf("UpdateKeyResult: %v", err)
	}
	if got.Title != title {
		t.Fatalf("Title=%q want %q", got.Title, title)
	}
	if got.TargetValue != target {
		t.Fatalf("TargetValue=%v want %v", got.TargetValue, target)
	}
	if got.Version != keyResult.Version+1 {
		t.Fatalf("Version=%d want %d", got.Version, keyResult.Version+1)
	}
	stored, err := repo.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Title != title {
		t.Fatalf("stored Title=%q want %q", stored.Title, title)
	}
}

func TestDeleteKeyResultDropsAggregate(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	repo := newFakeKeyResultRepository(keyResult)
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), repo)

	got, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: keyResult.ID, Note: "out of scope"})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	if got.Status != krdomain.KeyResultStatusDropped {
		t.Fatalf("Status=%q want %q", got.Status, krdomain.KeyResultStatusDropped)
	}
	stored, err := repo.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Status != krdomain.KeyResultStatusDropped {
		t.Fatalf("stored Status=%q want %q", stored.Status, krdomain.KeyResultStatusDropped)
	}
}

func TestDeleteKeyResultPublishesDroppedEventAfterPersistence(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusArchived)
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: keyResult.ID, Note: "out of scope"})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	if events.events[0].Type != string(KREventDropped) {
		t.Fatalf("event Type=%q want %q", events.events[0].Type, KREventDropped)
	}
	if events.events[0].Data["status"] != string(got.Status) {
		t.Fatalf("event status=%q want %q", events.events[0].Data["status"], got.Status)
	}
	if events.events[0].Data["note"] != "out of scope" {
		t.Fatalf("event note=%q want out of scope", events.events[0].Data["note"])
	}
}

func TestDeleteKeyResultRecomputesMissionToPausedWhenNoLiveKRsRemain(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	keyResult := krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen}
	missions := newFakeMissionRepository(mission)
	svc := newServiceForTest(t, missions, newFakeKeyResultRepository(keyResult))

	_, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: keyResult.ID})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusPaused {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusPaused)
	}
}

func TestDeleteKeyResultIsIdempotentWhenAlreadyDropped(t *testing.T) {
	keyResult := krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusDropped, Version: 4}
	events := &fakeEventPublisher{}
	svc := newServiceForTest(t, newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)), newFakeKeyResultRepository(keyResult))
	svc.events = events

	got, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: keyResult.ID, Note: "again"})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	if got.Status != krdomain.KeyResultStatusDropped || got.Version != keyResult.Version {
		t.Fatalf("got=%+v want dropped without version change", got)
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestDeleteKeyResultRecomputePublishesMissionLifecycleEvent(t *testing.T) {
	tests := []struct {
		name      string
		mission   missiondomain.Mission
		dropped   krdomain.KeyResult
		remaining krdomain.KeyResult
		eventKind MissionEventKind
	}{
		{
			name:      "paused when no live KRs remain",
			mission:   missionForServiceTest(t, missiondomain.MissionStatusActive),
			dropped:   krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusOpen},
			eventKind: MissionEventPaused,
		},
		{
			name:      "completed when all live KRs completed",
			mission:   missionForServiceTest(t, missiondomain.MissionStatusActive),
			dropped:   krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusOpen},
			remaining: krdomain.KeyResult{ID: "kr-2", MissionID: "mission-1", Status: krdomain.KeyResultStatusCompleted},
			eventKind: MissionEventCompleted,
		},
		{
			name:      "active when any live KR incomplete",
			mission:   missionForServiceTest(t, missiondomain.MissionStatusCompleted),
			dropped:   krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusCompleted},
			remaining: krdomain.KeyResult{ID: "kr-2", MissionID: "mission-1", Status: krdomain.KeyResultStatusInProgress},
			eventKind: MissionEventActivated,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events := &fakeEventPublisher{}
			keyResults := newFakeKeyResultRepository(tc.dropped)
			if tc.remaining.ID != "" {
				keyResults = newFakeKeyResultRepository(tc.dropped, tc.remaining)
			}
			svc, err := NewService(
				newFakeMissionRepository(tc.mission),
				keyResults,
				&fakeProgressEntryRepository{},
				&fakeLifecycleEventRepository{},
				fakeClock{now: serviceTestNow},
				fakeCallerResolver{},
				fakeTaskLoader{},
				&fakeLinkedTaskLoader{},
				events,
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			if _, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: tc.dropped.ID}); err != nil {
				t.Fatalf("DeleteKeyResult: %v", err)
			}
			if !hasEventType(events.events, tc.eventKind) {
				t.Fatalf("events missing %q: %+v", tc.eventKind, events.events)
			}
		})
	}
}

func TestDeleteKeyResultRecomputesMissionToCompletedWhenAllLiveKRsCompleted(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	dropped := krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen}
	completed := krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted}
	missions := newFakeMissionRepository(mission)
	svc := newServiceForTest(t, missions, newFakeKeyResultRepository(dropped, completed))

	_, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: dropped.ID})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusCompleted {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusCompleted)
	}
}

func TestDeleteKeyResultRecomputesMissionToActiveWhenAnyLiveKRIncomplete(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusCompleted)
	dropped := krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted}
	incomplete := krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusInProgress}
	missions := newFakeMissionRepository(mission)
	svc := newServiceForTest(t, missions, newFakeKeyResultRepository(dropped, incomplete))

	_, err := svc.DeleteKeyResult(context.Background(), DeleteKeyResultParams{KeyResultID: dropped.ID})
	if err != nil {
		t.Fatalf("DeleteKeyResult: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusActive {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusActive)
	}
}

func TestReopenKeyResultReactivatesCompletedMission(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusCompleted)
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.Status = krdomain.KeyResultStatusCompleted
	completedAt := serviceTestNow.Add(-30 * time.Minute)
	keyResult.CompletedAt = &completedAt
	missions := newFakeMissionRepository(mission)
	keyResults := newFakeKeyResultRepository(keyResult)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.ReopenKeyResult(context.Background(), ReopenKeyResultParams{KeyResultID: keyResult.ID})
	if err != nil {
		t.Fatalf("ReopenKeyResult: %v", err)
	}
	if got.Status != krdomain.KeyResultStatusOpen {
		t.Fatalf("Status=%q want %q", got.Status, krdomain.KeyResultStatusOpen)
	}
	storedMission, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if storedMission.Status != missiondomain.MissionStatusActive {
		t.Fatalf("mission Status=%q want %q", storedMission.Status, missiondomain.MissionStatusActive)
	}
	if !hasEventType(events.events, KREventReopened) {
		t.Fatalf("events missing %q: %+v", KREventReopened, events.events)
	}
	if !hasEventType(events.events, MissionEventActivated) {
		t.Fatalf("events missing %q: %+v", MissionEventActivated, events.events)
	}
}

func TestReopenKeyResultRejectsNonCompletedKeyResultBeforeWrite(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResults := newFakeKeyResultRepository(keyResult)
	events := &fakeEventPublisher{}
	svc := newServiceForTest(t, newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)), keyResults)
	svc.events = events

	if _, err := svc.ReopenKeyResult(context.Background(), ReopenKeyResultParams{KeyResultID: keyResult.ID}); err == nil {
		t.Fatal("ReopenKeyResult error is nil")
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Status != krdomain.KeyResultStatusDraft {
		t.Fatalf("stored Status=%q want %q", stored.Status, krdomain.KeyResultStatusDraft)
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestReopenKeyResultReopensDroppedAndResetsProgress(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusCompleted)
	baseline := 5.0
	keyResult := krdomain.KeyResult{
		ID:                    "kr-1",
		MissionID:             mission.ID,
		Title:                 "KR",
		MeasurementType:       krdomain.MeasurementNumber,
		Direction:             krdomain.DirectionIncrease,
		Baseline:              &baseline,
		TargetValue:           100,
		CurrentValue:          100,
		ProgressPercent:       100,
		LastMilestoneNotified: 100,
		Status:                krdomain.KeyResultStatusDropped,
		Version:               3,
	}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.ReopenKeyResult(context.Background(), ReopenKeyResultParams{KeyResultID: keyResult.ID, Note: "back in scope"})
	if err != nil {
		t.Fatalf("ReopenKeyResult: %v", err)
	}
	if got.Status != krdomain.KeyResultStatusOpen || got.CurrentValue != baseline || got.ProgressPercent != 0 {
		t.Fatalf("got status=%q current=%v progress=%d, want open baseline reset", got.Status, got.CurrentValue, got.ProgressPercent)
	}
	if got.LastUpdateNote != "back in scope" {
		t.Fatalf("LastUpdateNote=%q want back in scope", got.LastUpdateNote)
	}
	if !hasEventType(events.events, KREventReopened) {
		t.Fatalf("events missing %q: %+v", KREventReopened, events.events)
	}
}

func TestUpdateProgressPersistsKeyResultAndProgressEntry(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.LastMilestoneNotified = 50
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           50,
		Note:            "halfway",
		ExpectedVersion: keyResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.ProgressPercent != 50 || got.Status != krdomain.KeyResultStatusInProgress {
		t.Fatalf("got=%+v", got)
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.CurrentValue != 50 {
		t.Fatalf("stored CurrentValue=%v want 50", stored.CurrentValue)
	}
	if len(progressEntries.entries) != 1 {
		t.Fatalf("progress entries=%d want 1", len(progressEntries.entries))
	}
	if progressEntries.entries[0].ID == "" {
		t.Fatal("progress entry id is empty")
	}
}

func TestUpdateProgressPublishesProgressUpdatedEventAfterPersistence(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.LastMilestoneNotified = 50
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           50,
		ExpectedVersion: keyResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	event := events.events[0]
	if event.Type != string(KREventProgressUpdated) {
		t.Fatalf("event Type=%q want %q", event.Type, KREventProgressUpdated)
	}
	if event.Data["progressPercent"] != "50" {
		t.Fatalf("progressPercent=%q want 50", event.Data["progressPercent"])
	}
	if event.Data["status"] != string(got.Status) {
		t.Fatalf("status=%q want %q", event.Data["status"], got.Status)
	}
}

func TestUpdateProgressDoesNotPublishProgressEventWhenPersistenceFails(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{err: fmt.Errorf("write failed")},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           50,
		ExpectedVersion: keyResult.Version,
	}); err == nil {
		t.Fatal("UpdateProgress error is nil")
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestUpdateProgressCompletesKeyResultWhenLinkedTasksAreTerminal(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	linkedTasks := &fakeLinkedTaskLoader{taskIDs: []taskdomain.TaskID{"task-1", "task-2", "task-3"}}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoaderWithTasks{tasks: map[string]taskdomain.Task{
			"task-1": {ID: "task-1", Status: taskdomain.TaskStatusSucceeded},
			"task-2": {ID: "task-2", Status: taskdomain.TaskStatusFailed},
			"task-3": {ID: "task-3", Status: taskdomain.TaskStatusCanceled},
		}},
		linkedTasks,
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           100,
		ExpectedVersion: keyResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.Status != krdomain.KeyResultStatusCompleted {
		t.Fatalf("Status=%q want %q", got.Status, krdomain.KeyResultStatusCompleted)
	}
	if linkedTasks.calls != 1 {
		t.Fatalf("linked task calls=%d want 1", linkedTasks.calls)
	}
	if len(progressEntries.entries) != 1 {
		t.Fatalf("progress entries=%d want 1", len(progressEntries.entries))
	}
}

func TestUpdateProgressPublishesCompletedEventWhenProgressCompletesKeyResult(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.LastMilestoneNotified = 100
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoaderWithTasks{tasks: map[string]taskdomain.Task{
			"task-1": {ID: "task-1", Status: taskdomain.TaskStatusSucceeded},
		}},
		&fakeLinkedTaskLoader{taskIDs: []taskdomain.TaskID{"task-1"}},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           100,
		ExpectedVersion: keyResult.Version,
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if len(events.events) != 3 {
		t.Fatalf("events=%d want 3", len(events.events))
	}
	if events.events[0].Type != string(KREventProgressUpdated) {
		t.Fatalf("first event Type=%q want %q", events.events[0].Type, KREventProgressUpdated)
	}
	if events.events[1].Type != string(KREventCompleted) {
		t.Fatalf("second event Type=%q want %q", events.events[1].Type, KREventCompleted)
	}
	if events.events[2].Type != string(MissionEventCompleted) {
		t.Fatalf("third event Type=%q want %q", events.events[2].Type, MissionEventCompleted)
	}
	if events.events[1].Data["progressPercent"] != "100" {
		t.Fatalf("completed progressPercent=%q want 100", events.events[1].Data["progressPercent"])
	}
}

func TestUpdateProgressCompletingLastLiveKeyResultCompletesMission(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.LastMilestoneNotified = 100
	missions := newFakeMissionRepository(mission)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		newFakeKeyResultRepository(
			keyResult,
			krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted},
			krdomain.KeyResult{ID: "kr-dropped", MissionID: mission.ID, Status: krdomain.KeyResultStatusDropped},
		),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           100,
		ExpectedVersion: keyResult.Version,
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusCompleted {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusCompleted)
	}
	if !hasEventType(events.events, MissionEventCompleted) {
		t.Fatalf("events missing %q: %+v", MissionEventCompleted, events.events)
	}
}

func TestUpdateProgressRejectsDraftMissionBeforeWrite(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusDraft)
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           50,
		ExpectedVersion: keyResult.Version,
	})
	if err == nil {
		t.Fatal("UpdateProgress error is nil")
	}
	if len(progressEntries.entries) != 0 {
		t.Fatalf("progress entries=%d want 0", len(progressEntries.entries))
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Status != krdomain.KeyResultStatusDraft {
		t.Fatalf("key result Status=%q want %q", stored.Status, krdomain.KeyResultStatusDraft)
	}
	if stored.CurrentValue != 0 || stored.ProgressPercent != 0 || stored.Version != keyResult.Version {
		t.Fatalf("stored key result changed: %+v", stored)
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestUpdateProgressDoesNotCompleteMissionWhenAnotherLiveKeyResultIsIncomplete(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.LastMilestoneNotified = 100
	missions := newFakeMissionRepository(mission)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		newFakeKeyResultRepository(
			keyResult,
			krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusInProgress},
		),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           100,
		ExpectedVersion: keyResult.Version,
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusActive {
		t.Fatalf("mission Status=%q want %q", stored.Status, missiondomain.MissionStatusActive)
	}
	if hasEventType(events.events, MissionEventCompleted) {
		t.Fatalf("unexpected %q event: %+v", MissionEventCompleted, events.events)
	}
}

func TestUpdateProgressPublishesEveryNewlyCrossedMilestoneEvent(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.ProgressPercent = 10
	keyResult.LastMilestoneNotified = 0
	keyResults := newFakeKeyResultRepository(keyResult)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           80,
		ExpectedVersion: keyResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.LastMilestoneNotified != 75 {
		t.Fatalf("LastMilestoneNotified=%d want 75", got.LastMilestoneNotified)
	}
	gotMilestones := eventMilestones(events.events)
	wantMilestones := []string{"25", "50", "75"}
	if fmt.Sprint(gotMilestones) != fmt.Sprint(wantMilestones) {
		t.Fatalf("milestones=%v want %v", gotMilestones, wantMilestones)
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.LastMilestoneNotified != 75 {
		t.Fatalf("stored LastMilestoneNotified=%d want 75", stored.LastMilestoneNotified)
	}
}

func TestUpdateProgressDoesNotRefireAlreadyNotifiedMilestones(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.ProgressPercent = 40
	keyResult.LastMilestoneNotified = 50
	events := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           55,
		ExpectedVersion: keyResult.Version,
	})
	if err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if got.LastMilestoneNotified != 50 {
		t.Fatalf("LastMilestoneNotified=%d want 50", got.LastMilestoneNotified)
	}
	if gotMilestones := eventMilestones(events.events); len(gotMilestones) != 0 {
		t.Fatalf("milestones=%v want none", gotMilestones)
	}
}

func TestUpdateProgressRejectsKeyResultCompletionWhenLinkedTaskIsNonTerminalBeforeWrite(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoaderWithTasks{tasks: map[string]taskdomain.Task{
			"task-1": {ID: "task-1", Status: taskdomain.TaskStatusActive},
		}},
		&fakeLinkedTaskLoader{taskIDs: []taskdomain.TaskID{"task-1"}},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           100,
		ExpectedVersion: keyResult.Version,
	})
	if err == nil {
		t.Fatal("UpdateProgress error is nil")
	}
	if len(progressEntries.entries) != 0 {
		t.Fatalf("progress entries=%d want 0", len(progressEntries.entries))
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Status != krdomain.KeyResultStatusDraft {
		t.Fatalf("stored Status=%q want %q", stored.Status, krdomain.KeyResultStatusDraft)
	}
	if stored.CurrentValue != 0 {
		t.Fatalf("stored CurrentValue=%v want 0", stored.CurrentValue)
	}
}

func TestUpdateProgressBelowCompletionDoesNotLoadLinkedTasks(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	linkedTasks := &fakeLinkedTaskLoader{taskIDs: []taskdomain.TaskID{"task-1"}}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		newFakeKeyResultRepository(keyResult),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoaderWithTasks{tasks: map[string]taskdomain.Task{
			"task-1": {ID: "task-1", Status: taskdomain.TaskStatusActive},
		}},
		linkedTasks,
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     keyResult.ID,
		Value:           50,
		ExpectedVersion: keyResult.Version,
	}); err != nil {
		t.Fatalf("UpdateProgress: %v", err)
	}
	if linkedTasks.calls != 0 {
		t.Fatalf("linked task calls=%d want 0", linkedTasks.calls)
	}
}

func TestUpdateProgressRejectsCompletedKeyResultBeforeWrite(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.Status = krdomain.KeyResultStatusCompleted
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	svc, err := NewService(
		newFakeMissionRepository(missionForServiceTest(t, missiondomain.MissionStatusActive)),
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID: keyResult.ID,
		Value:       50,
	})
	if err == nil {
		t.Fatal("UpdateProgress error is nil")
	}
	if len(progressEntries.entries) != 0 {
		t.Fatalf("progress entries=%d want 0", len(progressEntries.entries))
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.CurrentValue != 0 {
		t.Fatalf("stored CurrentValue=%v want 0", stored.CurrentValue)
	}
}

func TestComputeMissionProgressAveragesNonDroppedKeyResults(t *testing.T) {
	keyResults := newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusCompleted, ProgressPercent: 100},
		krdomain.KeyResult{ID: "kr-2", MissionID: "mission-1", Status: krdomain.KeyResultStatusInProgress, ProgressPercent: 50},
		krdomain.KeyResult{ID: "kr-3", MissionID: "mission-1", Status: krdomain.KeyResultStatusDraft, ProgressPercent: 0},
		krdomain.KeyResult{ID: "kr-4", MissionID: "mission-1", Status: krdomain.KeyResultStatusDropped, ProgressPercent: 100},
	)
	svc := newServiceForTest(t, newFakeMissionRepository(), keyResults)

	got, err := svc.ComputeMissionProgress(context.Background(), missiondomain.MissionID("mission-1"))
	if err != nil {
		t.Fatalf("ComputeMissionProgress: %v", err)
	}
	if got.ProgressPercent != 50 {
		t.Fatalf("ProgressPercent=%d want 50", got.ProgressPercent)
	}
	if got.KeyResultCount != 3 {
		t.Fatalf("KeyResultCount=%d want 3", got.KeyResultCount)
	}
	if got.StatusCounts[krdomain.KeyResultStatusDropped] != 1 || got.StatusCounts[krdomain.KeyResultStatusInProgress] != 1 {
		t.Fatalf("StatusCounts=%v", got.StatusCounts)
	}
	if len(got.BlockingKeyResults) != 2 {
		t.Fatalf("BlockingKeyResults=%d want 2", len(got.BlockingKeyResults))
	}
}

func TestComputeMissionProgressNoNonDroppedKeyResults(t *testing.T) {
	keyResults := newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusDropped, ProgressPercent: 100},
	)
	svc := newServiceForTest(t, newFakeMissionRepository(), keyResults)

	got, err := svc.ComputeMissionProgress(context.Background(), missiondomain.MissionID("mission-1"))
	if err != nil {
		t.Fatalf("ComputeMissionProgress: %v", err)
	}
	if got.ProgressPercent != 0 {
		t.Fatalf("ProgressPercent=%d want 0", got.ProgressPercent)
	}
	if got.KeyResultCount != 0 {
		t.Fatalf("KeyResultCount=%d want 0", got.KeyResultCount)
	}
}

func TestUpdateMissionActivatesWithAtLeastOneLiveKeyResult(t *testing.T) {
	// A mission activates as long as it has one non-dropped key result; dropped
	// KRs are ignored and owner-project state is no longer consulted.
	mission := missionForServiceTest(t, missiondomain.MissionStatusDraft)
	keyResults := newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen},
		krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen},
		krdomain.KeyResult{ID: "kr-dropped", MissionID: mission.ID, Status: krdomain.KeyResultStatusDropped},
	)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), keyResults)
	status := missiondomain.MissionStatusActive

	got, err := svc.UpdateMission(context.Background(), UpdateMissionParams{
		MissionID: mission.ID,
		Status:    &status,
	})
	if err != nil {
		t.Fatalf("UpdateMission activate: %v", err)
	}
	if got.Status != missiondomain.MissionStatusActive {
		t.Fatalf("Status=%q want %q", got.Status, missiondomain.MissionStatusActive)
	}
}

func TestUpdateMissionActivationPublishesMissionActivatedEventAfterPersistence(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusDraft)
	missions := newFakeMissionRepository(mission)
	keyResults := newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen},
	)
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		keyResults,
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	status := missiondomain.MissionStatusActive

	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status}); err != nil {
		t.Fatalf("UpdateMission activate: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("events=%d want 1", len(events.events))
	}
	event := events.events[0]
	if event.Type != string(MissionEventActivated) {
		t.Fatalf("event Type=%q want %q", event.Type, MissionEventActivated)
	}
	if event.Message != string(MissionEventActivated) {
		t.Fatalf("event Message=%q want %q", event.Message, MissionEventActivated)
	}
	if event.RunID != types.RunID(mission.ID) {
		t.Fatalf("event RunID=%q want %q", event.RunID, mission.ID)
	}
	for key, want := range map[string]string{
		"missionId": string(mission.ID),
		"projectId": mission.ProjectID,
		"status":    string(missiondomain.MissionStatusActive),
	} {
		if event.Data[key] != want {
			t.Fatalf("event Data[%q]=%q want %q", key, event.Data[key], want)
		}
	}
	stored, err := missions.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if stored.Status != missiondomain.MissionStatusActive {
		t.Fatalf("stored Status=%q want %q", stored.Status, missiondomain.MissionStatusActive)
	}
}

func TestUpdateMissionActivationDoesNotPublishEventWhenPersistenceFails(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusDraft)
	missions := newFakeMissionRepository(mission)
	missions.updateErr = fmt.Errorf("write failed")
	events := &fakeEventPublisher{}
	svc, err := NewService(
		missions,
		newFakeKeyResultRepository(
			krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusOpen},
		),
		&fakeProgressEntryRepository{},
		&fakeLifecycleEventRepository{},
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		events,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	status := missiondomain.MissionStatusActive

	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status}); err == nil {
		t.Fatal("UpdateMission error is nil")
	}
	if len(events.events) != 0 {
		t.Fatalf("events=%d want 0", len(events.events))
	}
}

func TestUpdateMissionActivationRequiresNonDroppedKeyResult(t *testing.T) {
	// The activation gate was relaxed to a single precondition: at least one
	// non-dropped key result. The former KR->owner-project requirement (and its
	// "missing owner" / "closed owner" rejections) was removed, so a live KR now
	// activates regardless of any owner project's state.
	mission := missionForServiceTest(t, missiondomain.MissionStatusDraft)
	status := missiondomain.MissionStatusActive

	svc := newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-dropped", MissionID: mission.ID, Status: krdomain.KeyResultStatusDropped},
	))
	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status}); err == nil {
		t.Fatal("activate error is nil")
	}
}

func TestUpdateMissionCompletesWhenEveryLiveKeyResultCompleted(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted},
		krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted},
		krdomain.KeyResult{ID: "kr-dropped", MissionID: mission.ID, Status: krdomain.KeyResultStatusDropped},
	))
	status := missiondomain.MissionStatusCompleted

	got, err := svc.UpdateMission(context.Background(), UpdateMissionParams{
		MissionID: mission.ID,
		Status:    &status,
	})
	if err != nil {
		t.Fatalf("UpdateMission complete: %v", err)
	}
	if got.Status != missiondomain.MissionStatusCompleted {
		t.Fatalf("Status=%q want %q", got.Status, missiondomain.MissionStatusCompleted)
	}
}

func TestUpdateMissionPublishesLifecycleEventAfterPersistence(t *testing.T) {
	tests := []struct {
		name      string
		from      missiondomain.MissionStatus
		to        missiondomain.MissionStatus
		eventKind MissionEventKind
		keyResult krdomain.KeyResult
	}{
		{
			name:      "pause",
			from:      missiondomain.MissionStatusActive,
			to:        missiondomain.MissionStatusPaused,
			eventKind: MissionEventPaused,
		},
		{
			name:      "complete",
			from:      missiondomain.MissionStatusActive,
			to:        missiondomain.MissionStatusCompleted,
			eventKind: MissionEventCompleted,
			keyResult: krdomain.KeyResult{ID: "kr-1", MissionID: "mission-1", Status: krdomain.KeyResultStatusCompleted},
		},
		{
			name:      "archive",
			from:      missiondomain.MissionStatusActive,
			to:        missiondomain.MissionStatusArchived,
			eventKind: MissionEventArchived,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mission := missionForServiceTest(t, tc.from)
			keyResults := newFakeKeyResultRepository()
			if tc.keyResult.ID != "" {
				keyResults = newFakeKeyResultRepository(tc.keyResult)
			}
			events := &fakeEventPublisher{}
			svc, err := NewService(
				newFakeMissionRepository(mission),
				keyResults,
				&fakeProgressEntryRepository{},
				&fakeLifecycleEventRepository{},
				fakeClock{now: serviceTestNow},
				fakeCallerResolver{},
				fakeTaskLoader{},
				&fakeLinkedTaskLoader{},
				events,
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &tc.to}); err != nil {
				t.Fatalf("UpdateMission: %v", err)
			}
			if len(events.events) != 1 {
				t.Fatalf("events=%d want 1", len(events.events))
			}
			if events.events[0].Type != string(tc.eventKind) {
				t.Fatalf("event Type=%q want %q", events.events[0].Type, tc.eventKind)
			}
			if events.events[0].Data["status"] != string(tc.to) {
				t.Fatalf("event status=%q want %q", events.events[0].Data["status"], tc.to)
			}
		})
	}
}

func TestUpdateMissionCompletionRequiresAllLiveKeyResultsCompleted(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository(
		krdomain.KeyResult{ID: "kr-1", MissionID: mission.ID, Status: krdomain.KeyResultStatusCompleted},
		krdomain.KeyResult{ID: "kr-2", MissionID: mission.ID, Status: krdomain.KeyResultStatusInProgress},
	))
	status := missiondomain.MissionStatusCompleted

	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status}); err == nil {
		t.Fatal("complete error is nil")
	}
}

func TestUpdateMissionRejectsDraftTarget(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	svc := newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository())
	status := missiondomain.MissionStatusDraft

	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status}); err == nil {
		t.Fatal("draft target error is nil")
	}
}

func TestUpdateMissionLifecycleIsIdempotent(t *testing.T) {
	tests := []missiondomain.MissionStatus{
		missiondomain.MissionStatusActive,
		missiondomain.MissionStatusPaused,
		missiondomain.MissionStatusCompleted,
		missiondomain.MissionStatusArchived,
	}
	for _, status := range tests {
		t.Run(string(status), func(t *testing.T) {
			mission := missionForServiceTest(t, status)
			events := &fakeEventPublisher{}
			svc, err := NewService(
				newFakeMissionRepository(mission),
				newFakeKeyResultRepository(),
				&fakeProgressEntryRepository{},
				&fakeLifecycleEventRepository{},
				fakeClock{now: serviceTestNow},
				fakeCallerResolver{},
				fakeTaskLoader{},
				&fakeLinkedTaskLoader{},
				events,
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			got, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status})
			if err != nil {
				t.Fatalf("UpdateMission: %v", err)
			}
			if got.Status != status {
				t.Fatalf("Status=%q want %q", got.Status, status)
			}
			if len(events.events) != 0 {
				t.Fatalf("events=%d want 0", len(events.events))
			}
		})
	}
}

func TestGetLifecycleHistoryReturnsStoredNotes(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusActive)
	lifecycleEvents := &fakeLifecycleEventRepository{events: []types.EventRecord{
		{
			EventID:   "event-1",
			RunID:     types.RunID(mission.ID),
			CreatedAt: eventCreatedAt(serviceTestNow),
			Type:      string(MissionEventActivated),
			Message:   string(MissionEventActivated),
			Data: map[string]string{
				"missionId": string(mission.ID),
				"status":    string(missiondomain.MissionStatusActive),
				"note":      "kickoff",
			},
		},
		{
			EventID:   "event-2",
			RunID:     types.RunID(mission.ID),
			CreatedAt: eventCreatedAt(serviceTestNow.Add(time.Minute)),
			Type:      string(KREventDropped),
			Message:   string(KREventDropped),
			Data: map[string]string{
				"missionId":   string(mission.ID),
				"keyResultId": "kr-1",
				"status":      string(krdomain.KeyResultStatusDropped),
				"note":        "out of scope",
			},
		},
		{
			EventID:   "event-progress",
			RunID:     types.RunID(mission.ID),
			CreatedAt: eventCreatedAt(serviceTestNow.Add(2 * time.Minute)),
			Type:      string(KREventProgressUpdated),
			Message:   string(KREventProgressUpdated),
			Data:      map[string]string{"missionId": string(mission.ID)},
		},
	}}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		lifecycleEvents,
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := svc.GetLifecycleHistory(context.Background(), mission.ID, LifecycleHistoryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetLifecycleHistory: %v", err)
	}
	if got.MissionID != mission.ID || got.Count != 2 || len(got.Entries) != 2 {
		t.Fatalf("history=%+v want two lifecycle entries", got)
	}
	if got.Entries[0].Note != "kickoff" || got.Entries[0].Action != "activate" {
		t.Fatalf("first entry=%+v", got.Entries[0])
	}
	if got.Entries[1].Note != "out of scope" || got.Entries[1].KeyResultID != "kr-1" || got.Entries[1].Action != "kr_drop" {
		t.Fatalf("second entry=%+v", got.Entries[1])
	}
}

func TestIdempotentLifecycleNoteIsRecordedInHistoryOnly(t *testing.T) {
	mission := missionForServiceTest(t, missiondomain.MissionStatusCompleted)
	lifecycleEvents := &fakeLifecycleEventRepository{}
	published := &fakeEventPublisher{}
	svc, err := NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		lifecycleEvents,
		fakeClock{now: serviceTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		&fakeLinkedTaskLoader{},
		published,
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	status := missiondomain.MissionStatusCompleted
	if _, err := svc.UpdateMission(context.Background(), UpdateMissionParams{MissionID: mission.ID, Status: &status, Note: "confirmed complete"}); err != nil {
		t.Fatalf("UpdateMission: %v", err)
	}

	if len(published.events) != 0 {
		t.Fatalf("published events=%d want 0", len(published.events))
	}
	if len(lifecycleEvents.events) != 1 {
		t.Fatalf("history events=%d want 1", len(lifecycleEvents.events))
	}
	event := lifecycleEvents.events[0]
	if event.Type != string(MissionEventCompleted) || event.Data["note"] != "confirmed complete" || event.Data["idempotent"] != "true" {
		t.Fatalf("event=%+v", event)
	}
}

func missionForServiceTest(t *testing.T, status missiondomain.MissionStatus) missiondomain.Mission {
	t.Helper()
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       serviceTestNow.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	mission.Status = status
	return mission
}
