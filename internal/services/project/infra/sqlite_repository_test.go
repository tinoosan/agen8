package infra

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestSQLiteRepository_ProjectLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteProjectRepoForTest(t)
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
	repo := newSQLiteProjectRepoForTest(t)
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
}

func newSQLiteProjectRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle := newSQLiteHandleForTest(t)
	projects, err := NewSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return projects
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
