package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/config"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
	"github.com/tinoosan/agen8/internal/userctx"
)

const activeUserIDMarkerPath = "auth/active_user_id"

var (
	readDir    = os.ReadDir
	readFile   = os.ReadFile
	writeFile  = os.WriteFile
	removeFile = os.Remove
	mkdirAll   = os.MkdirAll
)

func EffectiveUserID(ctx context.Context, cfg config.Config) string {
	if userID := userctx.UserID(ctx); userID != "" {
		return userID
	}
	if userID := userIDFromDataDir(cfg.DataDir); userID != "" {
		return userID
	}
	if userID := activeUserIDFromDataDir(cfg.DataDir); userID != "" {
		return userID
	}
	if userID := activeSessionUserID(ctx, cfg); userID != "" {
		return userID
	}
	if userID := singleScopedUserID(cfg.DataDir); userID != "" {
		return userID
	}
	return userctx.LocalUserID
}

// ExplicitUserID resolves identity from explicit user-scoping signals only.
// It intentionally excludes the implicit local fallback so callers can fail
// closed when authentication is required.
func ExplicitUserID(ctx context.Context, cfg config.Config) string {
	if userID := userctx.UserID(ctx); userID != "" {
		if !strings.EqualFold(userID, userctx.LocalUserID) {
			return userID
		}
	}
	if userID := userIDFromDataDir(cfg.DataDir); userID != "" {
		return userID
	}
	if userID := activeUserIDFromDataDir(cfg.DataDir); userID != "" {
		return userID
	}
	if userID := activeSessionUserID(ctx, cfg); userID != "" {
		return userID
	}
	if userID := singleScopedUserID(cfg.DataDir); userID != "" {
		return userID
	}
	return ""
}

func HasRegisteredAuthUsers(ctx context.Context, cfg config.Config) (bool, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return false, nil
	}
	handle, err := GetDBHandle(ctx, cfg)
	if err != nil {
		return false, err
	}
	var count int
	if err := handle.DB().QueryRowContext(ctx, storagedb.Rebind(`SELECT COUNT(1) FROM auth_users`, handle.Dialect())).Scan(&count); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

func PersistActiveUserID(dataDir, userID string) error {
	dataDir = strings.TrimSpace(dataDir)
	userID = strings.TrimSpace(userID)
	if dataDir == "" {
		return errors.New("data dir is required")
	}
	if userID == "" {
		return errors.New("user id is required")
	}
	path := filepath.Join(dataDir, activeUserIDMarkerPath)
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFile(path, []byte(userID+"\n"), 0o600)
}

func ClearPersistedActiveUserID(dataDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil
	}
	path := filepath.Join(dataDir, activeUserIDMarkerPath)
	if err := removeFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func userIDFromDataDir(dataDir string) string {
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	if dataDir == "" {
		return ""
	}
	parent := filepath.Base(filepath.Dir(dataDir))
	if parent != "users" {
		return ""
	}
	return strings.TrimSpace(filepath.Base(dataDir))
}

func activeUserIDFromDataDir(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ""
	}
	raw, err := readFile(filepath.Join(dataDir, activeUserIDMarkerPath))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func singleScopedUserID(dataDir string) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return ""
	}
	entries, err := readDir(filepath.Join(dataDir, "users"))
	if err != nil {
		return ""
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := strings.TrimSpace(entry.Name())
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 {
		return ""
	}
	return ids[0]
}

func activeSessionUserID(ctx context.Context, cfg config.Config) string {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return ""
	}
	handle, err := GetDBHandle(ctx, cfg)
	if err != nil {
		return ""
	}
	rows, err := handle.DB().QueryContext(ctx, storagedb.Rebind(`
		SELECT TRIM(user_id)
		FROM auth_sessions
		WHERE TRIM(COALESCE(user_id, '')) <> ''
		  AND TRIM(COALESCE(expires_at, '')) <> ''
		  AND expires_at > ?
		ORDER BY created_at DESC, token DESC
		LIMIT 100
	`, handle.Dialect()), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return ""
	}
	defer rows.Close()

	resolved := ""
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return ""
		}
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if resolved == "" {
			resolved = userID
			continue
		}
		if !strings.EqualFold(resolved, userID) {
			// Ambiguous active sessions across users: fail closed and require
			// explicit context/marker/scoped dir instead of guessing.
			return ""
		}
	}
	if err := rows.Err(); err != nil {
		return ""
	}
	return resolved
}
