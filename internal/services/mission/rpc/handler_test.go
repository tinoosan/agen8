package rpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

var rpcTestNow = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

func TestNewHandlerRequiresService(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("panic is nil")
		}
	}()
	NewHandler(nil)
}

func TestCreateUsesMissionService(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	got, err := handler.Create(context.Background(), CreateMissionParams{
		ProjectID:   " project-1 ",
		Title:       " Mission ",
		Description: "Scope",
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-30",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Mission.ProjectID != "project-1" {
		t.Fatalf("ProjectID=%q want project-1", got.Mission.ProjectID)
	}
	if got.Mission.Title != "Mission" {
		t.Fatalf("Title=%q want Mission", got.Mission.Title)
	}
	if got.Mission.Status != string(missiondomain.MissionStatusDraft) {
		t.Fatalf("Status=%q want draft", got.Mission.Status)
	}
	if got.Mission.StartDate == "" || got.Mission.EndDate == "" || got.Mission.CreatedAt == "" || got.Mission.UpdatedAt == "" {
		t.Fatalf("Mission missing date fields: %+v", got.Mission)
	}
}

func TestCreateRejectsMissingProjectID(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.Create(context.Background(), CreateMissionParams{Title: "Mission"})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func TestCreateRejectsInvalidStartDate(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.Create(context.Background(), CreateMissionParams{
		ProjectID: "project-1",
		Title:     "Mission",
		StartDate: "01/06/2026",
	})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func TestListUsesMissionServiceFilter(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository()))

	got, err := handler.List(context.Background(), ListMissionsParams{
		ProjectID: " project-1 ",
		Status:    []string{"draft"},
		Limit:     25,
		Offset:    0,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got.Missions) != 1 || got.Missions[0].ID != "mission-1" {
		t.Fatalf("missions=%+v", got.Missions)
	}
}

func TestUpdateMissionUsesMissionService(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository()))
	title := "Updated"

	got, err := handler.Update(context.Background(), UpdateMissionParams{
		MissionID: " mission-1 ",
		Title:     &title,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Mission.Title != title {
		t.Fatalf("Title=%q want %q", got.Mission.Title, title)
	}
}

func TestUpdateMissionActivationReportsPreconditionAsInvalidParams(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	// A mission with no key results fails the only surviving activation
	// precondition; assert it surfaces as invalid params (-32602), not a 500.
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository()))
	status := string(missiondomain.MissionStatusActive)

	_, err = handler.Update(context.Background(), UpdateMissionParams{
		MissionID: "mission-1",
		Status:    &status,
	})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
	if !strings.Contains(err.Error(), "Add at least one key result before activating this mission.") {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestPurgeMissionUsesMissionService(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	repo := newFakeMissionRepository(mission)
	handler := NewHandler(newServiceForTest(t, repo, newFakeKeyResultRepository()))

	got, err := handler.Purge(context.Background(), PurgeMissionParams{MissionID: " mission-1 "})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if got.Mission.ID != string(mission.ID) {
		t.Fatalf("returned mission ID=%q want %q", got.Mission.ID, mission.ID)
	}
	if _, ok := repo.missions["mission-1"]; ok {
		t.Fatal("mission still present after purge")
	}
}

func TestPurgeMissionRejectsMissingID(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.Purge(context.Background(), PurgeMissionParams{MissionID: "  "})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func TestCreateKeyResultUsesMissionService(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository()))

	got, err := handler.CreateKeyResult(context.Background(), CreateKeyResultParams{
		MissionID:       " mission-1 ",
		Title:           " Revenue ",
		MeasurementType: " number ",
		Direction:       " increase ",
		TargetValue:     100,
	})
	if err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if got.KeyResult.MissionID != "mission-1" {
		t.Fatalf("MissionID=%q want mission-1", got.KeyResult.MissionID)
	}
	if got.KeyResult.MeasurementType != string(krdomain.MeasurementNumber) {
		t.Fatalf("MeasurementType=%q", got.KeyResult.MeasurementType)
	}
}

func TestUpdateKeyResultUsesMissionService(t *testing.T) {
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       missiondomain.MissionID("mission-1"),
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository(keyResult)))
	target := 200.0

	got, err := handler.UpdateKeyResult(context.Background(), UpdateKeyResultParams{
		KeyResultID: " kr-1 ",
		TargetValue: &target,
	})
	if err != nil {
		t.Fatalf("UpdateKeyResult: %v", err)
	}
	if got.KeyResult.TargetValue != target {
		t.Fatalf("TargetValue=%v want %v", got.KeyResult.TargetValue, target)
	}
}

func TestDeleteKeyResultRejectsMissingID(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.DeleteKeyResult(context.Background(), DeleteKeyResultParams{})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func TestReopenKeyResultUsesMissionService(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	mission.Status = missiondomain.MissionStatusCompleted
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     100,
		Now:             rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	keyResult.Status = krdomain.KeyResultStatusCompleted
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(mission), newFakeKeyResultRepository(keyResult)))

	got, err := handler.ReopenKeyResult(context.Background(), ReopenKeyResultParams{KeyResultID: " kr-1 "})
	if err != nil {
		t.Fatalf("ReopenKeyResult: %v", err)
	}
	if got.KeyResult.Status != string(krdomain.KeyResultStatusOpen) {
		t.Fatalf("Status=%q want %q", got.KeyResult.Status, krdomain.KeyResultStatusOpen)
	}
}

func TestUpdateProgressRejectsDraftMissionKeyResultWithoutWrite(t *testing.T) {
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID("mission-1"),
		ProjectID: "project-1",
		Title:     "Mission",
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	keyResult, err := krdomain.NewKeyResult(krdomain.NewKeyResultInput{
		ID:              krdomain.KeyResultID("kr-1"),
		MissionID:       mission.ID,
		Title:           "KR",
		MeasurementType: krdomain.MeasurementNumber,
		Direction:       krdomain.DirectionIncrease,
		TargetValue:     2,
		Now:             rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	missions := newFakeMissionRepository(mission)
	keyResults := newFakeKeyResultRepository(keyResult)
	progressEntries := &fakeProgressEntryRepository{}
	svc, err := missionapp.NewService(
		missions,
		keyResults,
		progressEntries,
		&fakeLifecycleEventRepository{},
		fakeClock{now: rpcTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(svc)

	_, err = handler.UpdateProgress(context.Background(), UpdateProgressParams{
		KeyResultID:     "kr-1",
		Value:           1,
		ExpectedVersion: keyResult.Version,
	})
	if err == nil {
		t.Fatal("UpdateProgress error is nil")
	}
	stored, err := keyResults.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if stored.Status != krdomain.KeyResultStatusDraft || stored.CurrentValue != 0 || stored.ProgressPercent != 0 || stored.Version != keyResult.Version {
		t.Fatalf("stored key result changed: %+v", stored)
	}
	if len(progressEntries.entries) != 0 {
		t.Fatalf("progress entries=%d want 0", len(progressEntries.entries))
	}
}

func TestProgressHistoryUsesMissionService(t *testing.T) {
	handler := NewHandler(newServiceForTestWithProgress(t, []krdomain.ProgressEntry{{
		ID:              "progress-1",
		KeyResultID:     krdomain.KeyResultID("kr-1"),
		PreviousValue:   10,
		NewValue:        25,
		ProgressPercent: 25,
		UpdatedBy:       "member-1",
		Note:            "quarter",
		CreatedAt:       rpcTestNow,
	}}))

	got, err := handler.ProgressHistory(context.Background(), ProgressHistoryParams{KeyResultID: " kr-1 "})
	if err != nil {
		t.Fatalf("ProgressHistory: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries=%+v", got.Entries)
	}
	entry := got.Entries[0]
	if entry.ID != "progress-1" || entry.NewValue != 25 || entry.ProgressPercent != 25 || entry.CreatedAt == "" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestProgressHistoryRejectsMissingID(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.ProgressHistory(context.Background(), ProgressHistoryParams{})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func TestLifecycleHistoryUsesMissionService(t *testing.T) {
	mission := rpcMission(t, "mission-1", "project-1")
	events := &fakeLifecycleEventRepository{events: []types.EventRecord{{
		EventID:   "event-1",
		RunID:     types.RunID(mission.ID),
		CreatedAt: rpcTestNow.UTC().Format(time.RFC3339Nano),
		Type:      string(missionapp.MissionEventActivated),
		Message:   string(missionapp.MissionEventActivated),
		Data: map[string]string{
			"missionId": string(mission.ID),
			"status":    "active",
			"note":      "ready",
		},
	}}}
	svc, err := missionapp.NewService(
		newFakeMissionRepository(mission),
		newFakeKeyResultRepository(),
		&fakeProgressEntryRepository{},
		events,
		fakeClock{now: rpcTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	handler := NewHandler(svc)

	got, err := handler.LifecycleHistory(context.Background(), LifecycleHistoryParams{MissionID: " mission-1 ", Limit: 5})
	if err != nil {
		t.Fatalf("LifecycleHistory: %v", err)
	}
	if got.MissionID != "mission-1" || got.Count != 1 || len(got.Entries) != 1 {
		t.Fatalf("history=%+v", got)
	}
	if got.Entries[0].Action != "activate" || got.Entries[0].Note != "ready" || got.Entries[0].Timestamp == "" {
		t.Fatalf("entry=%+v", got.Entries[0])
	}
}

func TestLifecycleHistoryRejectsMissingMissionID(t *testing.T) {
	handler := NewHandler(newServiceForTest(t, newFakeMissionRepository(), newFakeKeyResultRepository()))

	_, err := handler.LifecycleHistory(context.Background(), LifecycleHistoryParams{})
	if err == nil {
		t.Fatal("error is nil")
	}
	assertRPCCode(t, err, -32602)
}

func assertRPCCode(t *testing.T, err error, want int) {
	t.Helper()
	type rpcCoder interface {
		RPCCode() int
	}
	coded, ok := err.(rpcCoder)
	if !ok {
		t.Fatalf("error %T does not expose RPCCode", err)
	}
	if coded.RPCCode() != want {
		t.Fatalf("RPCCode=%d want %d", coded.RPCCode(), want)
	}
}

func newServiceForTest(t *testing.T, missions *fakeMissionRepository, keyResults *fakeKeyResultRepository) *missionapp.Service {
	t.Helper()
	return newServiceForTestWithProgressAndRepos(t, missions, keyResults, nil)
}

func newServiceForTestWithProgress(t *testing.T, entries []krdomain.ProgressEntry) *missionapp.Service {
	t.Helper()
	return newServiceForTestWithProgressAndRepos(t, newFakeMissionRepository(), newFakeKeyResultRepository(), entries)
}

func newServiceForTestWithProgressAndRepos(t *testing.T, missions *fakeMissionRepository, keyResults *fakeKeyResultRepository, entries []krdomain.ProgressEntry) *missionapp.Service {
	t.Helper()
	svc, err := missionapp.NewService(
		missions,
		keyResults,
		&fakeProgressEntryRepository{entries: entries},
		&fakeLifecycleEventRepository{},
		fakeClock{now: rpcTestNow},
		fakeCallerResolver{},
		fakeTaskLoader{},
		fakeLinkedTaskLoader{},
		&fakeEventPublisher{},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func rpcMission(t *testing.T, id string, projectID string) missiondomain.Mission {
	t.Helper()
	mission, err := missiondomain.NewMission(missiondomain.NewMissionInput{
		ID:        missiondomain.MissionID(id),
		ProjectID: projectID,
		Title:     "Mission " + id,
		Now:       rpcTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	return mission
}

type fakeMissionRepository struct {
	missions map[string]missiondomain.Mission
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
	if _, ok := r.missions[string(mission.ID)]; !ok {
		return fmt.Errorf("mission %s not found", mission.ID)
	}
	r.missions[string(mission.ID)] = mission
	return nil
}

func (r *fakeMissionRepository) DeleteMission(_ context.Context, missionID missiondomain.MissionID) error {
	delete(r.missions, string(missionID))
	return nil
}

type fakeKeyResultRepository struct {
	keyResults map[string]krdomain.KeyResult
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
	r.keyResults[string(keyResult.ID)] = keyResult
	return nil
}

func (r *fakeKeyResultRepository) UpdateKeyResult(_ context.Context, keyResult krdomain.KeyResult) error {
	if _, ok := r.keyResults[string(keyResult.ID)]; !ok {
		return fmt.Errorf("key result %s not found", keyResult.ID)
	}
	r.keyResults[string(keyResult.ID)] = keyResult
	return nil
}

type fakeProgressEntryRepository struct {
	entries []krdomain.ProgressEntry
}

func (r *fakeProgressEntryRepository) AppendProgressEntry(_ context.Context, entry krdomain.ProgressEntry) error {
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

func (fakeTaskLoader) Get(context.Context, missionapp.LinkedTaskID) (missionapp.LinkedTaskSnapshot, error) {
	return missionapp.LinkedTaskSnapshot{}, fmt.Errorf("task not found")
}

type fakeLinkedTaskLoader struct{}

func (fakeLinkedTaskLoader) ListTaskIDsForKeyResult(context.Context, krdomain.KeyResultID) ([]missionapp.LinkedTaskID, error) {
	return nil, nil
}

type fakeEventPublisher struct{}

func (*fakeEventPublisher) Append(context.Context, types.EventRecord) error {
	return nil
}

type fakeLifecycleEventRepository struct {
	events []types.EventRecord
}

func (r *fakeLifecycleEventRepository) AppendLifecycleEvent(_ context.Context, event types.EventRecord) error {
	r.events = append(r.events, event)
	return nil
}

func (r *fakeLifecycleEventRepository) ListLifecycleEvents(_ context.Context, missionID missiondomain.MissionID, filter missionapp.LifecycleHistoryFilter) ([]types.EventRecord, int, error) {
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
	return out, len(out), nil
}
