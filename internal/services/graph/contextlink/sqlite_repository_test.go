package contextlink

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

const migrationSQL = `
CREATE TABLE IF NOT EXISTS context_links (
    id TEXT PRIMARY KEY,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    edge_type TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 1.0,
    metadata_json TEXT DEFAULT '{}',
    created_at TEXT NOT NULL,
    created_by TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_cl_source ON context_links(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_cl_target ON context_links(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_cl_edge_type ON context_links(edge_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_unique_edge ON context_links(source_type, source_id, target_type, target_id, edge_type);
`

func setupRepository(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "graph-contextlink-test",
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, migrationSQL)
			return err
		},
	})
	require.NoError(t, err)
	repo, err := NewSQLiteRepository(handle)
	require.NoError(t, err)
	return repo
}

func TestSQLiteRepository_SaveFindAndDelete(t *testing.T) {
	repo := setupRepository(t)
	ctx := context.Background()
	link := Link{
		ID:         "cl-test-1",
		Source:     NodeRef{Type: "task", ID: "task-1"},
		Target:     NodeRef{Type: "key_result", ID: "kr-1"},
		EdgeType:   EdgeTypeServes,
		Confidence: 0.9,
		Metadata:   map[string]string{"rationale": "direct contribution"},
		CreatedBy:  "member:planner",
	}
	require.NoError(t, repo.Save(ctx, link))

	bySource, err := repo.FindBySource(ctx, NodeRef{Type: "task", ID: "task-1"})
	require.NoError(t, err)
	require.Len(t, bySource, 1)
	require.Equal(t, link.ID, bySource[0].ID)
	require.Equal(t, "direct contribution", bySource[0].Metadata["rationale"])

	byTarget, err := repo.FindByTarget(ctx, NodeRef{Type: "key_result", ID: "kr-1"})
	require.NoError(t, err)
	require.Len(t, byTarget, 1)
	require.Equal(t, NodeRef{Type: "task", ID: "task-1"}, byTarget[0].Source)

	byID, err := repo.FindByID(ctx, link.ID)
	require.NoError(t, err)
	require.Equal(t, link.ID, byID.ID)
	require.Equal(t, link.Target, byID.Target)

	require.NoError(t, repo.Delete(ctx, link.ID))
	_, err = repo.FindByID(ctx, link.ID)
	require.Error(t, err)
	bySource, err = repo.FindBySource(ctx, NodeRef{Type: "task", ID: "task-1"})
	require.NoError(t, err)
	require.Empty(t, bySource)
}

func TestSQLiteRepository_RejectsDuplicateSaveAndReplaces(t *testing.T) {
	repo := setupRepository(t)
	ctx := context.Background()
	base := Link{
		ID:         "cl-dup-1",
		Source:     NodeRef{Type: "decision", ID: "dec-1"},
		Target:     NodeRef{Type: "task", ID: "task-1"},
		EdgeType:   EdgeTypeProduced,
		Confidence: 0.6,
	}
	require.NoError(t, repo.Save(ctx, base))

	dup := base
	dup.ID = "cl-dup-2"
	dup.Confidence = 0.8
	require.Error(t, repo.Save(ctx, dup))

	require.NoError(t, repo.Replace(ctx, dup))
	found, err := repo.FindBetween(ctx, base.Source, base.Target)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, base.ID, found[0].ID)
	require.Equal(t, 0.8, found[0].Confidence)
}

func TestSQLiteRepository_DeleteLinksForEntity(t *testing.T) {
	repo := setupRepository(t)
	ctx := context.Background()
	links := []Link{
		{ID: "cl-1", Source: NodeRef{Type: "task", ID: "task-1"}, Target: NodeRef{Type: "decision", ID: "dec-1"}, EdgeType: EdgeTypeInformedBy, Confidence: 1},
		{ID: "cl-2", Source: NodeRef{Type: "decision", ID: "dec-2"}, Target: NodeRef{Type: "task", ID: "task-1"}, EdgeType: EdgeTypeProduced, Confidence: 1},
		{ID: "cl-3", Source: NodeRef{Type: "task", ID: "task-2"}, Target: NodeRef{Type: "decision", ID: "dec-3"}, EdgeType: EdgeTypeInformedBy, Confidence: 1},
	}
	for _, link := range links {
		require.NoError(t, repo.Save(ctx, link))
	}

	require.NoError(t, repo.DeleteLinksForEntity(ctx, NodeRef{Type: "task", ID: "task-1"}))
	remaining, err := repo.FindByEdgeType(ctx, EdgeTypeInformedBy, 10)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	require.Equal(t, ID("cl-3"), remaining[0].ID)
}

func TestLinkValidate(t *testing.T) {
	base := Link{Source: NodeRef{Type: "task", ID: "task-1"}, Target: NodeRef{Type: "decision", ID: "dec-1"}, EdgeType: EdgeTypeInformedBy, Confidence: 1}
	require.NoError(t, base.Validate())

	missing := base
	missing.Source.Type = ""
	require.Error(t, missing.Validate())

	self := base
	self.Target = self.Source
	require.Error(t, self.Validate())

	unknown := base
	unknown.EdgeType = "invented"
	require.Error(t, unknown.Validate())

	confidence := base
	confidence.Confidence = 1.1
	require.Error(t, confidence.Validate())
}
