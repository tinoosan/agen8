package infra_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRepositoryImplementationsSatisfyContract(t *testing.T) {
	var sqlite domain.SessionRepository = (*infra.SQLiteSessionRepository)(nil)
	var postgres domain.SessionRepository = (*infra.PostgresSessionRepository)(nil)
	var sqliteRun harnessrun.Repository = (*infra.SQLiteRunRepository)(nil)
	var postgresRun harnessrun.Repository = (*infra.PostgresRunRepository)(nil)

	assert.Nil(t, sqlite)
	assert.Nil(t, postgres)
	assert.Nil(t, sqliteRun)
	assert.Nil(t, postgresRun)
}

func TestPostgresSessionRepositoryRejectsWrongDriver(t *testing.T) {
	handle := sqliteHandleForRepositoryBuilderTest(t)

	_, err := infra.NewPostgresSessionRepository(handle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage driver must be postgres")
}

func TestRepositoryBuilderRejectsNilHandle(t *testing.T) {
	_, err := infra.NewRepository(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db handle is required")
}

func TestRepositoryBuilderSelectsSQLite(t *testing.T) {
	handle := sqliteHandleForRepositoryBuilderTest(t)

	repo, err := infra.NewRepository(handle)
	require.NoError(t, err)
	require.IsType(t, &infra.SQLiteSessionRepository{}, repo)
}

func TestRunRepositoryBuilderSelectsSQLite(t *testing.T) {
	handle := sqliteHandleForRepositoryBuilderTest(t)

	repo, err := infra.NewRunRepository(handle)
	require.NoError(t, err)
	require.IsType(t, &infra.SQLiteRunRepository{}, repo)
}

func sqliteHandleForRepositoryBuilderTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle := openSQLiteHandleForHarnessInfraTest(t)
	return handle
}
