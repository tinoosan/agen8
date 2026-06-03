package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/userctx"
)

func TestEffectiveUserID_PrefersContextUser(t *testing.T) {
	t.Parallel()

	ctx := userctx.WithUserID(context.Background(), "user-ctx")
	got := EffectiveUserID(ctx, config.Config{DataDir: t.TempDir()})
	if got != "user-ctx" {
		t.Fatalf("EffectiveUserID=%q want %q", got, "user-ctx")
	}
}

func TestEffectiveUserID_UsesScopedDataDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	dataDir := filepath.Join(base, "users", "user-scope")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir scoped data dir: %v", err)
	}
	got := EffectiveUserID(context.Background(), config.Config{DataDir: dataDir})
	if got != "user-scope" {
		t.Fatalf("EffectiveUserID=%q want %q", got, "user-scope")
	}
}

func TestEffectiveUserID_UsesPersistedActiveUser(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := PersistActiveUserID(base, "user-active"); err != nil {
		t.Fatalf("PersistActiveUserID: %v", err)
	}
	got := EffectiveUserID(context.Background(), config.Config{DataDir: base})
	if got != "user-active" {
		t.Fatalf("EffectiveUserID=%q want %q", got, "user-active")
	}
}

func TestEffectiveUserID_FallsBackToSingleScopedUser(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "users", "user-only"), 0o755); err != nil {
		t.Fatalf("mkdir users dir: %v", err)
	}
	got := EffectiveUserID(context.Background(), config.Config{DataDir: base})
	if got != "user-only" {
		t.Fatalf("EffectiveUserID=%q want %q", got, "user-only")
	}
}

func TestEffectiveUserID_FallsBackToLocalForMultipleUsers(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "users", "user-a"), 0o755); err != nil {
		t.Fatalf("mkdir users dir a: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(base, "users", "user-b"), 0o755); err != nil {
		t.Fatalf("mkdir users dir b: %v", err)
	}
	got := EffectiveUserID(context.Background(), config.Config{DataDir: base})
	if got != userctx.LocalUserID {
		t.Fatalf("EffectiveUserID=%q want %q", got, userctx.LocalUserID)
	}
}

func TestPersistAndClearActiveUserID(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := PersistActiveUserID(base, "user-1"); err != nil {
		t.Fatalf("PersistActiveUserID: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(base, "auth", "active_user_id"))
	if err != nil {
		t.Fatalf("read active user marker: %v", err)
	}
	if got := string(raw); got != "user-1\n" {
		t.Fatalf("marker=%q want %q", got, "user-1\n")
	}
	if err := ClearPersistedActiveUserID(base); err != nil {
		t.Fatalf("ClearPersistedActiveUserID: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "auth", "active_user_id")); !os.IsNotExist(err) {
		t.Fatalf("expected marker removed, stat err=%v", err)
	}
}

func TestEffectiveUserID_UsesActiveSessionWhenMarkerMissing(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	handle, err := GetDBHandle(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	db := handle.DB()
	ensureAuthSessionsTable(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO auth_sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		"token-one", "user-session", now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert auth session: %v", err)
	}

	got := EffectiveUserID(context.Background(), cfg)
	if got != "user-session" {
		t.Fatalf("EffectiveUserID=%q want %q", got, "user-session")
	}
}

func TestEffectiveUserID_IgnoresExpiredActiveSession(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	handle, err := GetDBHandle(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	db := handle.DB()
	ensureAuthSessionsTable(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO auth_sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		"token-expired", "user-expired", now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert expired auth session: %v", err)
	}

	got := EffectiveUserID(context.Background(), cfg)
	if got != userctx.LocalUserID {
		t.Fatalf("EffectiveUserID=%q want %q", got, userctx.LocalUserID)
	}
}

func TestEffectiveUserID_FailsClosedForAmbiguousActiveSessions(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	handle, err := GetDBHandle(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	db := handle.DB()
	ensureAuthSessionsTable(t, db)
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO auth_sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		"token-a", "user-a", now.Add(time.Hour).Format(time.RFC3339Nano), now.Add(-time.Minute).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert auth session user-a: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO auth_sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		"token-b", "user-b", now.Add(time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("insert auth session user-b: %v", err)
	}

	got := EffectiveUserID(context.Background(), cfg)
	if got != userctx.LocalUserID {
		t.Fatalf("EffectiveUserID=%q want %q", got, userctx.LocalUserID)
	}
}

func TestExplicitUserID_IgnoresFallbackLocalContextWhenMarkerExists(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	if err := PersistActiveUserID(base, "user-active"); err != nil {
		t.Fatalf("PersistActiveUserID: %v", err)
	}
	ctx := userctx.WithUserID(context.Background(), userctx.LocalUserID)
	got := ExplicitUserID(ctx, config.Config{DataDir: base})
	if got != "user-active" {
		t.Fatalf("ExplicitUserID=%q want %q", got, "user-active")
	}
}

func TestExplicitUserID_EmptyWhenOnlyImplicitLocal(t *testing.T) {
	t.Parallel()

	got := ExplicitUserID(context.Background(), config.Config{DataDir: t.TempDir()})
	if got != "" {
		t.Fatalf("ExplicitUserID=%q want empty", got)
	}
}

func TestHasRegisteredAuthUsers(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	handle, err := GetDBHandle(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	db := handle.DB()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auth_users (
			user_id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create auth_users table: %v", err)
	}
	hasUsers, err := HasRegisteredAuthUsers(context.Background(), cfg)
	if err != nil {
		t.Fatalf("HasRegisteredAuthUsers(empty): %v", err)
	}
	if hasUsers {
		t.Fatalf("HasRegisteredAuthUsers(empty)=%v want false", hasUsers)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO auth_users (user_id, email, name, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"user-1", "user@example.com", "User", "hash", now, now,
	); err != nil {
		t.Fatalf("insert auth user: %v", err)
	}
	hasUsers, err = HasRegisteredAuthUsers(context.Background(), cfg)
	if err != nil {
		t.Fatalf("HasRegisteredAuthUsers(with user): %v", err)
	}
	if !hasUsers {
		t.Fatalf("HasRegisteredAuthUsers(with user)=%v want true", hasUsers)
	}
}

func ensureAuthSessionsTable(t *testing.T, db interface {
	Exec(query string, args ...any) (sql.Result, error)
}) {
	t.Helper()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auth_sessions (
			token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatalf("create auth_sessions table: %v", err)
	}
}
