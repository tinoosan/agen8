package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	"github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	"github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

var infraTestNow = time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

func TestSQLiteRepositoryPersistsMission(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	mission := infraMission(t, "mission-1", "project-1")

	if err := repo.CreateMission(context.Background(), mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	loaded, err := repo.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission: %v", err)
	}
	if loaded.ID != mission.ID || loaded.Title != mission.Title {
		t.Fatalf("loaded mission=%+v want %+v", loaded, mission)
	}

	updated, err := loaded.UpdateDetails("Updated", "Updated scope", infraTestNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("UpdateDetails: %v", err)
	}
	if err := repo.UpdateMission(context.Background(), updated); err != nil {
		t.Fatalf("UpdateMission: %v", err)
	}
	loaded, err = repo.GetMission(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("GetMission updated: %v", err)
	}
	if loaded.Title != "Updated" {
		t.Fatalf("Title=%q want Updated", loaded.Title)
	}
}

func TestSQLiteRepositoryListsMissionsByProjectAndStatus(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	first := infraMission(t, "mission-1", "project-1")
	second := infraMission(t, "mission-2", "project-2")
	if err := repo.CreateMission(context.Background(), first); err != nil {
		t.Fatalf("CreateMission first: %v", err)
	}
	if err := repo.CreateMission(context.Background(), second); err != nil {
		t.Fatalf("CreateMission second: %v", err)
	}

	listed, err := repo.ListMissions(context.Background(), mission.MissionFilter{
		ProjectID: "project-1",
		Statuses:  []mission.MissionStatus{mission.MissionStatusDraft},
	})
	if err != nil {
		t.Fatalf("ListMissions: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != first.ID {
		t.Fatalf("listed=%+v want %s", listed, first.ID)
	}
}

func TestSQLiteRepositoryCutsOverLegacyMissionTables(t *testing.T) {
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			if driver != storagedb.DriverSQLite {
				t.Fatalf("driver=%q", driver)
			}
			for _, stmt := range []string{
				`CREATE TABLE missions (
					id TEXT PRIMARY KEY,
					project_id TEXT NOT NULL,
					title TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'draft',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
				`CREATE INDEX idx_missions_project ON missions(project_id)`,
				`CREATE TABLE key_results (
					id TEXT PRIMARY KEY,
					mission_id TEXT NOT NULL,
					title TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'open',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
				`CREATE INDEX idx_key_results_mission ON key_results(mission_id)`,
			} {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	listed, err := repo.ListMissions(context.Background(), mission.MissionFilter{ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("ListMissions: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed=%+v want empty rebuilt table", listed)
	}
	mission := infraMission(t, "mission-1", "project-1")
	if err := repo.CreateMission(context.Background(), mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
}

func TestSQLiteRepositoryPersistsKeyResultAndProgress(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	mission := infraMission(t, "mission-1", "project-1")
	if err := repo.CreateMission(context.Background(), mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	keyResult := infraKeyResult(t, "kr-1", mission.ID)
	if err := repo.CreateKeyResult(context.Background(), keyResult); err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	loaded, err := repo.GetKeyResult(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("GetKeyResult: %v", err)
	}
	if loaded.ID != keyResult.ID || loaded.MissionID != mission.ID {
		t.Fatalf("loaded key result=%+v want %+v", loaded, keyResult)
	}
	listed, err := repo.ListKeyResults(context.Background(), mission.ID)
	if err != nil {
		t.Fatalf("ListKeyResults: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != keyResult.ID {
		t.Fatalf("listed=%+v want %s", listed, keyResult.ID)
	}

	entry := kr.ProgressEntry{
		ID:              "progress-1",
		KeyResultID:     keyResult.ID,
		PreviousValue:   0,
		NewValue:        50,
		ProgressPercent: 50,
		UpdatedBy:       "member-1",
		Note:            "halfway",
		CreatedAt:       infraTestNow,
	}
	if err := repo.AppendProgressEntry(context.Background(), entry); err != nil {
		t.Fatalf("AppendProgressEntry: %v", err)
	}
	entries, err := repo.ListProgressEntries(context.Background(), keyResult.ID)
	if err != nil {
		t.Fatalf("ListProgressEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("entries=%+v want %s", entries, entry.ID)
	}
}

func TestSQLiteRepositoryPersistsLifecycleEvents(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	mission := infraMission(t, "mission-1", "project-1")
	if err := repo.CreateMission(context.Background(), mission); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	first := types.EventRecord{
		EventID:   "event-1",
		RunID:     types.RunID(mission.ID),
		CreatedAt: timeString(infraTestNow),
		Type:      string(missionapp.MissionEventActivated),
		Message:   string(missionapp.MissionEventActivated),
		Data: map[string]string{
			"missionId": string(mission.ID),
			"status":    "active",
			"note":      "ready",
		},
	}
	if err := repo.AppendLifecycleEvent(context.Background(), first); err != nil {
		t.Fatalf("AppendLifecycleEvent first: %v", err)
	}
	second := types.EventRecord{
		EventID:   "event-2",
		RunID:     types.RunID(mission.ID),
		CreatedAt: timeString(infraTestNow.Add(time.Minute)),
		Type:      string(missionapp.KREventDropped),
		Message:   string(missionapp.KREventDropped),
		Data: map[string]string{
			"missionId":   string(mission.ID),
			"keyResultId": "kr-1",
			"status":      string(kr.KeyResultStatusDropped),
			"note":        "out of scope",
		},
	}
	if err := repo.AppendLifecycleEvent(context.Background(), second); err != nil {
		t.Fatalf("AppendLifecycleEvent second: %v", err)
	}

	events, count, err := repo.ListLifecycleEvents(context.Background(), mission.ID, missionapp.LifecycleHistoryFilter{
		Limit: 1,
		Types: []string{string(missionapp.KREventDropped)},
	})
	if err != nil {
		t.Fatalf("ListLifecycleEvents: %v", err)
	}
	if count != 1 || len(events) != 1 {
		t.Fatalf("events=%+v count=%d want one dropped event", events, count)
	}
	if events[0].Data["note"] != "out of scope" || events[0].Data["keyResultId"] != "kr-1" {
		t.Fatalf("event=%+v", events[0])
	}
}

func TestSQLiteRepositoryDeleteMissionCascadesDescendants(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	ctx := context.Background()

	// Two missions so we can prove the cascade only touches the target.
	target := infraMission(t, "mission-1", "project-1")
	bystander := infraMission(t, "mission-2", "project-1")
	for _, m := range []mission.Mission{target, bystander} {
		if err := repo.CreateMission(ctx, m); err != nil {
			t.Fatalf("CreateMission %s: %v", m.ID, err)
		}
	}

	// Give the target a KR with a progress entry and a lifecycle event.
	keyResult := infraKeyResult(t, "kr-1", target.ID)
	if err := repo.CreateKeyResult(ctx, keyResult); err != nil {
		t.Fatalf("CreateKeyResult: %v", err)
	}
	if err := repo.AppendProgressEntry(ctx, kr.ProgressEntry{
		ID:              "progress-1",
		KeyResultID:     keyResult.ID,
		NewValue:        50,
		ProgressPercent: 50,
		UpdatedBy:       "member-1",
		CreatedAt:       infraTestNow,
	}); err != nil {
		t.Fatalf("AppendProgressEntry: %v", err)
	}
	if err := repo.AppendLifecycleEvent(ctx, types.EventRecord{
		EventID:   "event-1",
		RunID:     types.RunID(target.ID),
		CreatedAt: timeString(infraTestNow),
		Type:      string(missionapp.MissionEventActivated),
		Message:   string(missionapp.MissionEventActivated),
		Data:      map[string]string{"missionId": string(target.ID), "status": "active"},
	}); err != nil {
		t.Fatalf("AppendLifecycleEvent: %v", err)
	}

	// And give the bystander its own KR so we can prove it survives.
	bystanderKR := infraKeyResult(t, "kr-2", bystander.ID)
	if err := repo.CreateKeyResult(ctx, bystanderKR); err != nil {
		t.Fatalf("CreateKeyResult bystander: %v", err)
	}

	if err := repo.DeleteMission(ctx, target.ID); err != nil {
		t.Fatalf("DeleteMission: %v", err)
	}

	// Target and all of its descendants must be gone.
	if _, err := repo.GetMission(ctx, target.ID); err == nil {
		t.Fatal("target mission still present after delete")
	}
	if krs, err := repo.ListKeyResults(ctx, target.ID); err != nil {
		t.Fatalf("ListKeyResults target: %v", err)
	} else if len(krs) != 0 {
		t.Fatalf("target key results=%d want 0", len(krs))
	}
	if entries, err := repo.ListProgressEntries(ctx, keyResult.ID); err != nil {
		t.Fatalf("ListProgressEntries: %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("progress entries=%d want 0", len(entries))
	}
	if events, count, err := repo.ListLifecycleEvents(ctx, target.ID, missionapp.LifecycleHistoryFilter{}); err != nil {
		t.Fatalf("ListLifecycleEvents: %v", err)
	} else if count != 0 || len(events) != 0 {
		t.Fatalf("lifecycle events=%d count=%d want 0", len(events), count)
	}

	// The bystander mission and its KR must be untouched.
	if _, err := repo.GetMission(ctx, bystander.ID); err != nil {
		t.Fatalf("bystander mission removed by cascade: %v", err)
	}
	if krs, err := repo.ListKeyResults(ctx, bystander.ID); err != nil {
		t.Fatalf("ListKeyResults bystander: %v", err)
	} else if len(krs) != 1 || krs[0].ID != bystanderKR.ID {
		t.Fatalf("bystander key results=%+v want [%s]", krs, bystanderKR.ID)
	}
}

func newSQLiteRepositoryForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func infraMission(t *testing.T, id string, projectID string) mission.Mission {
	t.Helper()
	out, err := mission.NewMission(mission.NewMissionInput{
		ID:        mission.MissionID(id),
		ProjectID: projectID,
		Title:     "Mission " + id,
		Now:       infraTestNow,
	})
	if err != nil {
		t.Fatalf("NewMission: %v", err)
	}
	return out
}

func infraKeyResult(t *testing.T, id string, missionID mission.MissionID) kr.KeyResult {
	t.Helper()
	out, err := kr.NewKeyResult(kr.NewKeyResultInput{
		ID:              kr.KeyResultID(id),
		MissionID:       missionID,
		Title:           "KR " + id,
		MeasurementType: kr.MeasurementNumber,
		Direction:       kr.DirectionIncrease,
		TargetValue:     100,
		Now:             infraTestNow,
	})
	if err != nil {
		t.Fatalf("NewKeyResult: %v", err)
	}
	return out
}
