package infra

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestSQLiteRepository_ProjectLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newSQLiteProjectReposForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	saved, err := repo.Save(ctx, project.Record{
		ID:         "project-1",
		LocationID: "local",
		Root:       "/tmp/project-1",
		Title:      "Project One",
		Status:     project.StatusOpen,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID != "project-1" || saved.LocationID != "local" || saved.Root != "/tmp/project-1" || saved.Status != project.StatusOpen {
		t.Fatalf("saved project = %+v", saved)
	}

	loaded, err := repo.Get(ctx, "project-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.Title != "Project One" {
		t.Fatalf("loaded project = %+v", loaded)
	}

	updated := loaded
	updated.Title = "Renamed"
	updated.UpdatedAt = now.Add(time.Minute)
	if _, err := repo.Save(ctx, updated); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	listed, err := repo.List(ctx, project.Filter{Status: project.StatusOpen})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Title != "Renamed" {
		t.Fatalf("listed projects = %+v", listed)
	}
}

func TestSQLiteRepository_ProjectValidationAndNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newSQLiteProjectReposForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	if _, err := repo.Save(ctx, project.Record{ID: "project-1", CreatedAt: now}); err == nil {
		t.Fatalf("expected missing root error")
	}
	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Get missing error = %v", err)
	}
	if err := repo.Delete(ctx, "missing"); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Delete missing error = %v", err)
	}
}

func TestSQLiteClusterRepository_ClusterAndSpaceRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, clusters := newSQLiteProjectReposForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	saved, err := clusters.Save(ctx, cluster.Record{
		ID:        "cluster-1",
		ProjectID: "project-1",
		Name:      "Launch",
		Status:    cluster.StatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("Save cluster: %v", err)
	}
	if saved.ID != "cluster-1" || saved.ProjectID != "project-1" {
		t.Fatalf("saved cluster = %+v", saved)
	}

	if _, err := clusters.SaveSpace(ctx, cluster.SpaceRefRecord{
		ClusterID: "cluster-1",
		SpaceID:   "space-2",
		SortOrder: 2,
		Pinned:    false,
	}); err != nil {
		t.Fatalf("SaveSpace space-2: %v", err)
	}
	if _, err := clusters.SaveSpace(ctx, cluster.SpaceRefRecord{
		ClusterID: "cluster-1",
		SpaceID:   "space-1",
		SortOrder: 1,
		Pinned:    true,
	}); err != nil {
		t.Fatalf("SaveSpace space-1: %v", err)
	}

	listed, err := clusters.List(ctx, cluster.Filter{ProjectID: "project-1", Status: cluster.StatusOpen})
	if err != nil {
		t.Fatalf("List clusters: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "Launch" {
		t.Fatalf("listed clusters = %+v", listed)
	}
	spaces, err := clusters.ListSpaces(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("ListSpaces: %v", err)
	}
	if len(spaces) != 2 || spaces[0].SpaceID != "space-1" || !spaces[0].Pinned {
		t.Fatalf("cluster spaces = %+v", spaces)
	}

	if err := clusters.RemoveSpace(ctx, "cluster-1", "space-1"); err != nil {
		t.Fatalf("RemoveSpace: %v", err)
	}
	spaces, err = clusters.ListSpaces(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("ListSpaces after remove: %v", err)
	}
	if len(spaces) != 1 || spaces[0].SpaceID != "space-2" {
		t.Fatalf("cluster spaces after remove = %+v", spaces)
	}
}

func TestSQLiteClusterRepository_ValidationAndNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, clusters := newSQLiteProjectReposForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	if _, err := clusters.Save(ctx, cluster.Record{ID: "cluster-1", ProjectID: "project-1", CreatedAt: now}); err == nil {
		t.Fatalf("expected missing cluster name error")
	}
	if _, err := clusters.SaveSpace(ctx, cluster.SpaceRefRecord{ClusterID: "cluster-1"}); err == nil {
		t.Fatalf("expected missing space id error")
	}
	if err := clusters.RemoveSpace(ctx, "cluster-1", "missing"); !errors.Is(err, cluster.ErrNotFound) {
		t.Fatalf("RemoveSpace missing error = %v", err)
	}
}

func TestRepositoryBuildersSelectSQLiteFromHandle(t *testing.T) {
	t.Parallel()
	handle := newSQLiteHandleForTest(t)

	projects, err := NewRepository(handle)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	if _, ok := projects.(*SQLiteRepository); !ok {
		t.Fatalf("NewRepository type = %T, want *SQLiteRepository", projects)
	}

	clusters, err := NewClusterRepository(handle)
	if err != nil {
		t.Fatalf("NewClusterRepository: %v", err)
	}
	if _, ok := clusters.(*SQLiteClusterRepository); !ok {
		t.Fatalf("NewClusterRepository type = %T, want *SQLiteClusterRepository", clusters)
	}
}

func newSQLiteProjectReposForTest(t *testing.T) (*SQLiteRepository, *SQLiteClusterRepository) {
	t.Helper()
	handle := newSQLiteHandleForTest(t)
	projects, err := NewSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	clusters, err := NewSQLiteClusterRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteClusterRepository: %v", err)
	}
	return projects, clusters
}

func newSQLiteHandleForTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return handle
}
