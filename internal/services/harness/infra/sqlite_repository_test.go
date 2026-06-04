package infra_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var testNow = time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

func setupTestDB(t *testing.T) *infra.SQLiteSessionRepository {
	t.Helper()
	handle := openSQLiteHandleForHarnessInfraTest(t)

	repo, err := infra.NewSQLiteSessionRepository(handle)
	require.NoError(t, err)
	return repo
}

func openSQLiteHandleForHarnessInfraTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "harness-session-test",
		Migrate: func(_ context.Context, db *sql.DB, _ storagedb.Driver) error {
			return infra.MigrateSessionSchema(context.Background(), db)
		},
	})
	require.NoError(t, err)
	return handle
}

func runtimeContext(memberID, spaceID, kind, model, effort string) domain.RuntimeContext {
	return domain.RuntimeContext{
		ProjectID:      "project-1",
		LocationID:     "local",
		MemberID:       memberID,
		SpaceID:        spaceID,
		ChannelID:      "channel:" + spaceID + ":member:" + memberID,
		DisplayName:    "Worker One",
		MemberType:     "worker",
		LifecycleState: "active",
		HarnessKind:    kind,
		Model:          model,
		Effort:         effort,
		Workdir:        "/tmp/project-1",
		SystemPrompt:   "prompt",
		MCPToken:       "token-" + memberID,
		MCPServers:     []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=token-` + memberID + `"`},
	}
}

func TestSQLiteSessionRepository_SaveAndGet(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	session, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, session.UpdateClaudeChannelURL("http://127.0.0.1:4567/notify"))

	require.NoError(t, repo.Save(ctx, session))

	got, err := repo.Get(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.ID)
	assert.Equal(t, "member-1", got.MemberID)
	assert.Equal(t, "project-1", got.ProjectID)
	assert.Equal(t, "local", got.LocationID)
	assert.Equal(t, "space-1", got.SpaceID)
	assert.Equal(t, domain.SessionActive, got.Status)
	assert.Equal(t, "claude-cli", got.Kind)
	assert.Equal(t, "claude-opus-4-7", got.Model)
	assert.Equal(t, "high", got.Effort)
	assert.Equal(t, "/tmp/project-1", got.Workdir)
	assert.Equal(t, "channel:space-1:member:member-1", got.ChannelID)
	assert.Equal(t, "Worker One", got.DisplayName)
	assert.Equal(t, "prompt", got.SystemPrompt)
	assert.Equal(t, "token-member-1", got.MCPToken)
	assert.Equal(t, []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=token-member-1"`}, got.MCPServers)
	assert.Equal(t, "http://127.0.0.1:4567/notify", got.ClaudeChannelURL)
	assert.Equal(t, int64(0), got.TokensIn)
}

func TestSQLiteSessionRepository_GetNotFound(t *testing.T) {
	repo := setupTestDB(t)
	got, err := repo.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSQLiteSessionRepository_SaveUpdates(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	session, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, session))

	session.AddUsage(100, 50)
	deactivatedAt := testNow.Add(time.Hour)
	require.NoError(t, session.Deactivate(domain.ReasonShutdown, "", deactivatedAt))
	require.NoError(t, repo.Save(ctx, session))

	got, err := repo.Get(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.SessionInactive, got.Status)
	assert.Equal(t, domain.ReasonShutdown, got.InactiveReason)
	assert.Equal(t, int64(100), got.TokensIn)
	assert.Equal(t, int64(50), got.TokensOut)
	require.NotNil(t, got.DeactivatedAt)
}

func TestSQLiteSessionRepository_GetActiveByMember(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	s1, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s1))

	got, err := repo.GetActiveByMember(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sess-1", got.ID)

	require.NoError(t, s1.Deactivate(domain.ReasonShutdown, "", testNow.Add(time.Hour)))
	require.NoError(t, repo.Save(ctx, s1))

	got, err = repo.GetActiveByMember(ctx, "member-1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSQLiteSessionRepository_ListBySpace(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	s1, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s1))

	s2, err := domain.NewSession("sess-2", runtimeContext("member-2", "space-1", "codex", "gpt-5.5", "medium"), testNow.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s2))

	s3, err := domain.NewSession("sess-3", runtimeContext("member-3", "space-2", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s3))

	sessions, err := repo.ListBySpace(ctx, "space-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	sessions, err = repo.ListBySpace(ctx, "space-2")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	sessions, err = repo.ListBySpace(ctx, "space-3")
	require.NoError(t, err)
	assert.Len(t, sessions, 0)
}

func TestSQLiteSessionRepository_ListByMember(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	s1, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s1))

	s2, err := domain.NewSession("sess-2", runtimeContext("member-1", "space-2", "codex", "gpt-5.5", "medium"), testNow.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s2))

	s3, err := domain.NewSession("sess-3", runtimeContext("member-2", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	require.NoError(t, repo.Save(ctx, s3))

	sessions, err := repo.ListByMember(ctx, "member-1")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, "sess-2", sessions[0].ID)
	assert.Equal(t, "sess-1", sessions[1].ID)

	sessions, err = repo.ListByMember(ctx, "member-3")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestSQLiteSessionRepository_InvalidTimestampFailsLoudly(t *testing.T) {
	ctx := context.Background()
	handle := openSQLiteHandleForHarnessInfraTest(t)
	repo, err := infra.NewSQLiteSessionRepository(handle)
	require.NoError(t, err)

	_, err = repo.Get(ctx, "sess-bad")
	require.NoError(t, err)

	_, err = handle.DB().ExecContext(ctx, `
		INSERT INTO harness_sessions (
			session_id, project_id, member_id, space_id, status, inactive_reason, inactive_error,
			activated_at, tokens_in, tokens_out, harness_kind, model, effort, session_ref,
			channel_id, display_name, member_type, lifecycle_state, system_prompt, mcp_token, mcp_servers_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-bad", "project-1", "member-1", "space-1", "active", "", "", "not-a-time", 0, 0, "codex", "gpt-5.5", "high", "",
		"channel:space-1:member:member-1", "Worker One", "worker", "active", "prompt", "token", "[]",
	)
	require.NoError(t, err)

	_, err = repo.Get(ctx, "sess-bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activated_at")
}

func TestSQLiteSessionRepository_NilHandle(t *testing.T) {
	_, err := infra.NewSQLiteSessionRepository(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handle is required")
}

func TestMigrateSessionSchema_NilDB(t *testing.T) {
	err := infra.MigrateSessionSchema(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db is nil")
}
