package infra_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func setupRunRepo(t *testing.T) harnessrun.Repository {
	t.Helper()
	handle := openSQLiteHandleForHarnessRunInfraTest(t)
	repo, err := infra.NewSQLiteRunRepository(handle)
	require.NoError(t, err)
	return repo
}

func openSQLiteHandleForHarnessRunInfraTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "harness-run-test",
		Migrate: func(_ context.Context, db *sql.DB, _ storagedb.Driver) error {
			return infra.MigrateRunSchema(context.Background(), db)
		},
	})
	require.NoError(t, err)
	return handle
}

func TestRunRepositorySaveAndGet(t *testing.T) {
	repo := setupRunRepo(t)
	ctx := context.Background()
	item := newRun(t, "run-1", "session-1", "turn-1", testNow)

	require.NoError(t, repo.Save(ctx, item))

	got, err := repo.Get(ctx, "run-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, item.ID, got.ID)
	assert.Equal(t, harnessrun.StatusRunning, got.Status)
	assert.Equal(t, testNow, got.StartedAt)
}

func TestRunRepositoryUpdatesAndFindsByTurnID(t *testing.T) {
	repo := setupRunRepo(t)
	ctx := context.Background()
	item := newRun(t, "run-1", "session-1", "turn-local", testNow)
	require.NoError(t, item.SetNativeTurnID("turn-native"))
	require.NoError(t, item.RequestStop("user-1", testNow.Add(time.Minute)))
	require.NoError(t, item.MarkCanceled(testNow.Add(2*time.Minute)))
	require.NoError(t, repo.Save(ctx, item))

	for _, turnID := range []string{"turn-local", "turn-native"} {
		got, err := repo.GetByTurnID(ctx, turnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "run-1", got.ID)
		assert.Equal(t, harnessrun.StatusCanceled, got.Status)
		require.NotNil(t, got.CompletedAt)
	}
}

func TestRunRepositoryGetActiveBySession(t *testing.T) {
	repo := setupRunRepo(t)
	ctx := context.Background()
	item := newRun(t, "run-1", "session-1", "turn-1", testNow)
	require.NoError(t, repo.Save(ctx, item))

	got, err := repo.GetActiveBySession(ctx, "session-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "run-1", got.ID)

	require.NoError(t, item.MarkCompleted(testNow.Add(time.Minute)))
	require.NoError(t, repo.Save(ctx, item))
	got, err = repo.GetActiveBySession(ctx, "session-1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRunRepositoryListFilters(t *testing.T) {
	repo := setupRunRepo(t)
	ctx := context.Background()
	first := newRun(t, "run-1", "session-1", "turn-1", testNow)
	second := newRun(t, "run-2", "session-2", "turn-2", testNow.Add(time.Minute))
	second.MemberID = "member-2"
	require.NoError(t, second.MarkFailed("boom", testNow.Add(2*time.Minute)))
	require.NoError(t, repo.Save(ctx, first))
	require.NoError(t, repo.Save(ctx, second))

	rows, err := repo.List(ctx, harnessrun.Filter{ProjectID: "project-1", Status: []harnessrun.Status{harnessrun.StatusFailed}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "run-2", rows[0].ID)

	rows, err = repo.List(ctx, harnessrun.Filter{MemberID: "member-1"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "run-1", rows[0].ID)
}

func TestRunRepositoryRejectsInvalidStatusFilter(t *testing.T) {
	repo := setupRunRepo(t)
	_, err := repo.List(context.Background(), harnessrun.Filter{Status: []harnessrun.Status{"bogus"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run status")
}

func newRun(t *testing.T, id, sessionID, turnID string, startedAt time.Time) harnessrun.Run {
	t.Helper()
	item, err := harnessrun.Start(harnessrun.StartParams{
		ID:               id,
		ProjectID:        "project-1",
		SpaceID:          "space-1",
		ChannelID:        "channel-1",
		MemberID:         "member-1",
		SessionID:        sessionID,
		HarnessKind:      "codex",
		NativeSessionRef: "thread-1",
		TurnID:           turnID,
		StartedAt:        startedAt,
	})
	require.NoError(t, err)
	return item
}
