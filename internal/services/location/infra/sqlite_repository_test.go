package infra

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func TestSQLiteRepository_LocationLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteLocationRepoForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	probedAt := now.Add(time.Minute)

	saved, err := repo.Save(ctx, locationdomain.Record{
		ID:     "loc_ssh",
		Kind:   locationdomain.KindSSH,
		Label:  "Remote",
		Status: locationdomain.StatusOnline,
		Ready:  true,
		Address: locationdomain.Address{
			Host:     "example.internal",
			Port:     22,
			Username: "agent",
		},
		CredentialRef: "cred_1",
		Probe: locationdomain.Probe{
			Reachable:    true,
			FileBrowsing: true,
			Exec:         true,
			Codex:        true,
			Claude:       true,
		},
		LastProbedAt: &probedAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID != "loc_ssh" || saved.Address.Host != "example.internal" || !saved.Ready || !saved.Probe.Codex || !saved.Probe.Claude {
		t.Fatalf("saved location = %+v", saved)
	}

	loaded, err := repo.Get(ctx, "loc_ssh")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.CredentialRef != "cred_1" || loaded.LastProbedAt == nil || !loaded.LastProbedAt.Equal(probedAt) {
		t.Fatalf("loaded location = %+v", loaded)
	}

	updated := loaded
	updated.Label = "Renamed"
	updated.Ready = false
	updated.Status = locationdomain.StatusNotReady
	updated.UpdatedAt = now.Add(2 * time.Minute)
	if _, err := repo.Save(ctx, updated); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	ready := false
	listed, err := repo.List(ctx, locationdomain.Filter{
		Kind:   locationdomain.KindSSH,
		Status: locationdomain.StatusNotReady,
		Ready:  &ready,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Label != "Renamed" {
		t.Fatalf("listed locations = %+v", listed)
	}
}

func TestSQLiteRepository_ValidationAndNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLiteLocationRepoForTest(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	if _, err := repo.Save(ctx, locationdomain.Record{
		ID:        "loc_ssh",
		Kind:      locationdomain.KindSSH,
		Status:    locationdomain.StatusOffline,
		CreatedAt: now,
		UpdatedAt: now,
	}); err == nil {
		t.Fatalf("expected missing ssh address error")
	}
	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, locationdomain.ErrNotFound) {
		t.Fatalf("Get missing error = %v", err)
	}
	if err := repo.Delete(ctx, "missing"); !errors.Is(err, locationdomain.ErrNotFound) {
		t.Fatalf("Delete missing error = %v", err)
	}
}

func TestRepositoryBuilderSelectsSQLiteFromHandle(t *testing.T) {
	t.Parallel()
	handle := newSQLiteHandleForTest(t)
	repo, err := NewRepository(handle)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	if _, ok := repo.(*SQLiteRepository); !ok {
		t.Fatalf("NewRepository type = %T, want *SQLiteRepository", repo)
	}
}

func newSQLiteLocationRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, err := NewSQLiteRepository(newSQLiteHandleForTest(t))
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
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
