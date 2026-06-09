package infra

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

var userInfraNow = time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

func TestSQLiteRepositoryCreateGetAndCount(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	record := userRecord(t, "user-1", "ADMIN@EXAMPLE.COM")

	if err := repo.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}
	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
	got, err := repo.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID.String() != "user-1" {
		t.Fatalf("id=%q want user-1", got.ID.String())
	}
	if got.Email != "admin@example.com" {
		t.Fatalf("email=%q want normalized admin@example.com", got.Email)
	}
	if got.Role != user.RoleAdmin {
		t.Fatalf("role=%q want admin", got.Role)
	}
}

func TestSQLiteRepositoryGetByEmailNormalizes(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	record := userRecord(t, "user-1", "admin@example.com")
	if err := repo.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByEmail(context.Background(), " ADMIN@example.COM ")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID.String() != "user-1" {
		t.Fatalf("id=%q want user-1", got.ID.String())
	}
}

func TestSQLiteRepositoryUpdatePersistsLifecycle(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	record := userRecord(t, "user-1", "admin@example.com")
	if err := repo.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}

	record.Name = "Admin Updated"
	record.Lifecycle = user.LifecycleSuspended
	record.UpdatedAt = userInfraNow.Add(time.Minute)
	if err := repo.Update(context.Background(), record); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Admin Updated" {
		t.Fatalf("name=%q want Admin Updated", got.Name)
	}
	if got.Lifecycle != user.LifecycleSuspended {
		t.Fatalf("lifecycle=%q want suspended", got.Lifecycle)
	}
}

func TestSQLiteRepositoryPersistsPreferences(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	record := userRecord(t, "user-1", "admin@example.com")
	record.Preferences = user.Preferences{
		Theme:              "nebula",
		LastDarkTheme:      "nebula",
		LastLightTheme:     "sepia",
		DefaultProjectView: "strategy",
		FontFamily:         "fraunces",
		FontScale:          18,
	}
	if err := repo.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Preferences.Theme != "nebula" {
		t.Fatalf("theme=%q want nebula", got.Preferences.Theme)
	}
	if got.Preferences.DefaultProjectView != "strategy" {
		t.Fatalf("default project view=%q want strategy", got.Preferences.DefaultProjectView)
	}
	if got.Preferences.FontFamily != "fraunces" {
		t.Fatalf("font family=%q want fraunces", got.Preferences.FontFamily)
	}
	if got.Preferences.FontScale != 18 {
		t.Fatalf("font scale=%d want 18", got.Preferences.FontScale)
	}
}

func TestSQLiteRepositoryMissingUserFailsLoudly(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	id, err := user.NewID("missing-user")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	_, err = repo.Get(context.Background(), id)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestSQLiteRepositoryDuplicateEmailFailsLoudly(t *testing.T) {
	repo := newSQLiteRepositoryForTest(t)
	if err := repo.Create(context.Background(), userRecord(t, "user-1", "admin@example.com")); err != nil {
		t.Fatalf("Create user-1: %v", err)
	}
	err := repo.Create(context.Background(), userRecord(t, "user-2", " ADMIN@example.COM "))
	if !errors.Is(err, user.ErrEmailAlreadyExists) {
		t.Fatalf("err=%v want ErrEmailAlreadyExists", err)
	}
}

func newSQLiteRepositoryForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(context.Context, *sql.DB, storagedb.Driver) error {
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

func userRecord(t *testing.T, rawID string, email string) user.User {
	t.Helper()
	id, err := user.NewID(rawID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	record, err := user.New(user.NewInput{
		ID:    id,
		Email: email,
		Name:  "Admin",
		Role:  user.RoleAdmin,
		Now:   userInfraNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return record
}
