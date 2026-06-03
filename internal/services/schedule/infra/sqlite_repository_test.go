package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var infraTestNow = time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

func TestSQLiteRepositoryPersistsEntriesAndDueRuns(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	entry := infraEntry(t, "schedule-1", infraTestNow.Add(time.Minute))
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create: %v", err)
	}
	loaded, err := repo.Get(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.ID != entry.ID || loaded.Title != entry.Title {
		t.Fatalf("loaded=%+v want %+v", loaded, entry)
	}
	due, err := repo.ListDue(context.Background(), infraTestNow, 10)
	if err != nil {
		t.Fatalf("ListDue before due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("due before due=%+v want empty", due)
	}
	due, err = repo.ListDue(context.Background(), infraTestNow.Add(2*time.Minute), 10)
	if err != nil {
		t.Fatalf("ListDue after due: %v", err)
	}
	if len(due) != 1 || due[0].ID != entry.ID {
		t.Fatalf("due=%+v want %s", due, entry.ID)
	}

	run, err := schedule.NewStartedRun(entry, *entry.NextRunAt, infraTestNow.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("NewStartedRun: %v", err)
	}
	claimed, ok, err := repo.ClaimDue(context.Background(), run)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if !ok || claimed.ID != run.ID {
		t.Fatalf("claim ok=%v run=%+v want %s", ok, claimed, run.ID)
	}
	claimedAgain, ok, err := repo.ClaimDue(context.Background(), run)
	if err != nil {
		t.Fatalf("ClaimDue duplicate: %v", err)
	}
	if ok || claimedAgain.ID != run.ID {
		t.Fatalf("duplicate claim ok=%v run=%+v want existing %s", ok, claimedAgain, run.ID)
	}
}

func TestSQLiteRepositoryUpdatesEntryAndRun(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	entry := infraEntry(t, "schedule-1", infraTestNow.Add(time.Minute))
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create: %v", err)
	}
	cancelled, err := entry.Cancel(infraTestNow.Add(30 * time.Second))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := repo.Update(context.Background(), cancelled); err != nil {
		t.Fatalf("Update: %v", err)
	}
	listed, err := repo.List(context.Background(), schedule.Filter{
		SpaceID: spacedomain.SpaceID("space-1"),
		Status:  schedule.EntryStatusCancelled,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Status != schedule.EntryStatusCancelled {
		t.Fatalf("listed=%+v want cancelled entry", listed)
	}

	run, err := schedule.NewStartedRun(entry, *entry.NextRunAt, infraTestNow.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("NewStartedRun: %v", err)
	}
	claimed, ok, err := repo.ClaimDue(context.Background(), run)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	if !ok {
		t.Fatalf("ClaimDue ok=false, want true")
	}
	succeeded, err := claimed.Succeed("task-1", infraTestNow.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("Succeed: %v", err)
	}
	if err := repo.UpdateRun(context.Background(), succeeded); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	runs, err := repo.ListRuns(context.Background(), schedule.RunFilter{EntryID: entry.ID})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != schedule.RunStatusSucceeded || runs[0].TargetObjectID != "task-1" {
		t.Fatalf("runs=%+v want succeeded task run", runs)
	}
}

func TestSQLiteRepositoryCutsOverLegacyScheduleTable(t *testing.T) {
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			if driver != storagedb.DriverSQLite {
				t.Fatalf("driver=%q", driver)
			}
			_, err := db.ExecContext(ctx, `CREATE TABLE schedule_entries (
				entry_id TEXT PRIMARY KEY,
				space_id TEXT NOT NULL,
				role_id TEXT NOT NULL,
				created_by TEXT NOT NULL,
				name TEXT NOT NULL,
				goal TEXT NOT NULL
			)`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	entry := infraEntry(t, "schedule-1", infraTestNow.Add(time.Minute))
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func newSQLiteRepositoryForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
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

func infraEntry(t *testing.T, id schedule.EntryID, runAt time.Time) schedule.Entry {
	t.Helper()
	entry, err := schedule.NewEntry(schedule.NewEntryInput{
		ID:        id,
		SpaceID:   spacedomain.SpaceID("space-1"),
		CreatedBy: schedule.ActorRef("member-1"),
		Title:     "Check admission status",
		Timing:    schedule.TimingExpression{Mode: schedule.TimingModeOnce, RunAt: &runAt},
		Target: schedule.Target{
			Kind: schedule.TargetKindTaskCreate,
			TaskCreate: schedule.TaskCreatePayload{
				TargetMemberID: member.ID("worker-1"),
				Title:          "Check admission status",
				Description:    "Look for a status update",
			},
		},
	}, infraTestNow)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return entry
}
