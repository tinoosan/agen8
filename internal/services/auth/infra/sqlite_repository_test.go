package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	auth "github.com/tinoosan/agen8-mcp-server/internal/services/auth/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/password"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/session"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var authInfraNow = time.Date(2026, 5, 17, 17, 0, 0, 0, time.UTC)

func TestSQLiteRepositoriesPasswordSaveGetDelete(t *testing.T) {
	repos := newSQLiteAuthRepositoriesForTest(t)
	userID := authUserID(t, "user-1")
	credential, err := password.New(password.NewInput{
		UserID:       userID,
		PasswordHash: "hashed-password",
		Now:          authInfraNow,
	})
	if err != nil {
		t.Fatalf("password New: %v", err)
	}

	if err := repos.Passwords.Save(context.Background(), credential); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repos.Passwords.Get(context.Background(), userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PasswordHash != "hashed-password" {
		t.Fatalf("password hash=%q want hashed-password", got.PasswordHash)
	}
	if err := repos.Passwords.Delete(context.Background(), userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repos.Passwords.Get(context.Background(), userID); err == nil {
		t.Fatal("expected deleted password credential to be missing")
	}
}

func TestSQLiteRepositoriesSessionCreateGetUpdate(t *testing.T) {
	repos := newSQLiteAuthRepositoriesForTest(t)
	sessionID, err := session.NewID("session-1")
	if err != nil {
		t.Fatalf("session NewID: %v", err)
	}
	record := session.Session{
		ID:        sessionID,
		UserID:    authUserID(t, "user-1"),
		TokenHash: auth.HashToken("session-token"),
		ExpiresAt: authInfraNow.Add(time.Hour),
		CreatedAt: authInfraNow,
	}

	if err := repos.Sessions.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.Sessions.GetByTokenHash(context.Background(), record.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got.ID.String() != "session-1" {
		t.Fatalf("session id=%q want session-1", got.ID.String())
	}
	revokedAt := authInfraNow.Add(time.Minute)
	got.RevokedAt = &revokedAt
	if err := repos.Sessions.Update(context.Background(), got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repos.Sessions.GetByTokenHash(context.Background(), record.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash updated: %v", err)
	}
	if updated.RevokedAt == nil || !updated.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revokedAt=%v want %v", updated.RevokedAt, revokedAt)
	}
}

func TestSQLiteRepositoriesAPIKeyCreateGetUpdate(t *testing.T) {
	repos := newSQLiteAuthRepositoriesForTest(t)
	keyID, err := apikey.NewID("api-key-1")
	if err != nil {
		t.Fatalf("apikey NewID: %v", err)
	}
	record := apikey.Key{
		ID:        keyID,
		UserID:    authUserID(t, "user-1"),
		Name:      "CLI",
		Prefix:    "ak_abc",
		TokenHash: auth.HashToken("api-token"),
		CreatedAt: authInfraNow,
	}

	if err := repos.APIKeys.Create(context.Background(), record); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.APIKeys.GetByTokenHash(context.Background(), record.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got.Name != "CLI" {
		t.Fatalf("name=%q want CLI", got.Name)
	}
	revokedAt := authInfraNow.Add(time.Minute)
	got.RevokedAt = &revokedAt
	if err := repos.APIKeys.Update(context.Background(), got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repos.APIKeys.Get(context.Background(), keyID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.RevokedAt == nil || !updated.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revokedAt=%v want %v", updated.RevokedAt, revokedAt)
	}
}

func TestSQLiteRepositoriesAPIKeyListByUser(t *testing.T) {
	repos := newSQLiteAuthRepositoriesForTest(t)
	userA := authUserID(t, "user-a")
	userB := authUserID(t, "user-b")
	for _, record := range []apikey.Key{
		apiKeyRecord(t, "api-key-1", userA, "first", authInfraNow),
		apiKeyRecord(t, "api-key-2", userA, "second", authInfraNow.Add(time.Minute)),
		apiKeyRecord(t, "api-key-3", userB, "other", authInfraNow.Add(2*time.Minute)),
	} {
		if err := repos.APIKeys.Create(context.Background(), record); err != nil {
			t.Fatalf("Create %s: %v", record.ID.String(), err)
		}
	}

	keys, err := repos.APIKeys.ListByUser(context.Background(), userA)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("len(keys)=%d want 2", len(keys))
	}
	if keys[0].Name != "second" || keys[1].Name != "first" {
		t.Fatalf("key order=%q,%q want newest first", keys[0].Name, keys[1].Name)
	}
	for _, key := range keys {
		if key.UserID != userA {
			t.Fatalf("listed key for wrong user: %+v", key)
		}
	}
}

func newSQLiteAuthRepositoriesForTest(t *testing.T) Repositories {
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
	repos, err := NewRepositories(handle)
	if err != nil {
		t.Fatalf("NewRepositories: %v", err)
	}
	return repos
}

func apiKeyRecord(t *testing.T, rawID string, userID user.ID, name string, createdAt time.Time) apikey.Key {
	t.Helper()
	keyID, err := apikey.NewID(rawID)
	if err != nil {
		t.Fatalf("apikey NewID: %v", err)
	}
	return apikey.Key{
		ID:        keyID,
		UserID:    userID,
		Name:      name,
		Prefix:    "ak_" + rawID,
		TokenHash: auth.HashToken(rawID),
		CreatedAt: createdAt,
	}
}

func authUserID(t *testing.T, raw string) user.ID {
	t.Helper()
	id, err := user.NewID(raw)
	if err != nil {
		t.Fatalf("user NewID: %v", err)
	}
	return id
}
