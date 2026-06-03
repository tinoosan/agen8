package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/apikey"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/password"
	"github.com/tinoosan/agen8-mcp-server/internal/services/auth/session"
	user "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type sqlStore struct {
	db      *sql.DB
	driver  storagedb.Driver
	dialect storagedb.Dialect
}

func newSQLStore(handle *storagedb.Handle) *sqlStore {
	return &sqlStore{
		db:      handle.DB(),
		driver:  handle.Driver(),
		dialect: handle.Dialect(),
	}
}

func (s *sqlStore) Get(ctx context.Context, userID user.ID) (password.Credential, error) {
	if userID.IsZero() {
		return password.Credential{}, fmt.Errorf("user id is required")
	}
	var rawUserID, passwordHash, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT user_id, password_hash, created_at, updated_at
		FROM auth_passwords
		WHERE user_id = ?
	`), userID.String()).Scan(&rawUserID, &passwordHash, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return password.Credential{}, fmt.Errorf("password credential not found")
		}
		return password.Credential{}, fmt.Errorf("query password credential: %w", err)
	}
	id, err := user.NewID(rawUserID)
	if err != nil {
		return password.Credential{}, fmt.Errorf("scan password user id: %w", err)
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return password.Credential{}, fmt.Errorf("scan password created at: %w", err)
	}
	updated, err := parseTime(updatedAt)
	if err != nil {
		return password.Credential{}, fmt.Errorf("scan password updated at: %w", err)
	}
	return password.Credential{
		UserID:       id,
		PasswordHash: passwordHash,
		CreatedAt:    created,
		UpdatedAt:    updated,
	}, nil
}

func (s *sqlStore) Save(ctx context.Context, credential password.Credential) error {
	if credential.UserID.IsZero() {
		return fmt.Errorf("user id is required")
	}
	if strings.TrimSpace(credential.PasswordHash) == "" {
		return fmt.Errorf("password hash is required")
	}
	if credential.CreatedAt.IsZero() {
		return fmt.Errorf("password created at is required")
	}
	if credential.UpdatedAt.IsZero() {
		return fmt.Errorf("password updated at is required")
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO auth_passwords (user_id, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			password_hash = excluded.password_hash,
			updated_at = excluded.updated_at
	`),
		credential.UserID.String(),
		strings.TrimSpace(credential.PasswordHash),
		timeString(credential.CreatedAt),
		timeString(credential.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("save password credential: %w", err)
	}
	return nil
}

func (s *sqlStore) Delete(ctx context.Context, userID user.ID) error {
	if userID.IsZero() {
		return fmt.Errorf("user id is required")
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM auth_passwords WHERE user_id = ?`), userID.String())
	if err != nil {
		return fmt.Errorf("delete password credential: %w", err)
	}
	return nil
}

func (s *sqlStore) GetByTokenHash(ctx context.Context, tokenHash string) (session.Session, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return session.Session{}, fmt.Errorf("session token hash is required")
	}
	var rawID, rawUserID, hash, expiresAt, createdAt string
	var revokedAt sql.NullString
	err := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT session_id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM auth_sessions
		WHERE token_hash = ?
	`), tokenHash).Scan(&rawID, &rawUserID, &hash, &expiresAt, &revokedAt, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.Session{}, fmt.Errorf("session not found")
		}
		return session.Session{}, fmt.Errorf("query session: %w", err)
	}
	return scanSession(rawID, rawUserID, hash, expiresAt, revokedAt, createdAt)
}

func (s *sqlStore) Create(ctx context.Context, record session.Session) error {
	if err := validateSession(record); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO auth_sessions (session_id, user_id, token_hash, expires_at, revoked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`),
		record.ID.String(),
		record.UserID.String(),
		strings.TrimSpace(record.TokenHash),
		timeString(record.ExpiresAt),
		nullableTimeString(record.RevokedAt),
		timeString(record.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *sqlStore) Update(ctx context.Context, record session.Session) error {
	if err := validateSession(record); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE auth_sessions
		SET user_id = ?, token_hash = ?, expires_at = ?, revoked_at = ?
		WHERE session_id = ?
	`),
		record.UserID.String(),
		strings.TrimSpace(record.TokenHash),
		timeString(record.ExpiresAt),
		nullableTimeString(record.RevokedAt),
		record.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	return requireAffected(result, "session")
}

func (s *sqlStore) GetAPIKeyByTokenHash(ctx context.Context, tokenHash string) (apikey.Key, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return apikey.Key{}, fmt.Errorf("api key token hash is required")
	}
	return s.queryAPIKey(ctx, `
		SELECT api_key_id, user_id, name, prefix, token_hash, expires_at, revoked_at, created_at
		FROM auth_api_keys
		WHERE token_hash = ?
	`, tokenHash)
}

func (s *sqlStore) GetAPIKey(ctx context.Context, id apikey.ID) (apikey.Key, error) {
	if strings.TrimSpace(id.String()) == "" {
		return apikey.Key{}, fmt.Errorf("api key id is required")
	}
	return s.queryAPIKey(ctx, `
		SELECT api_key_id, user_id, name, prefix, token_hash, expires_at, revoked_at, created_at
		FROM auth_api_keys
		WHERE api_key_id = ?
	`, id.String())
}

func (s *sqlStore) CreateAPIKey(ctx context.Context, record apikey.Key) error {
	if err := validateAPIKey(record); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO auth_api_keys (api_key_id, user_id, name, prefix, token_hash, expires_at, revoked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`),
		record.ID.String(),
		record.UserID.String(),
		strings.TrimSpace(record.Name),
		strings.TrimSpace(record.Prefix),
		strings.TrimSpace(record.TokenHash),
		nullableTimeString(record.ExpiresAt),
		nullableTimeString(record.RevokedAt),
		timeString(record.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (s *sqlStore) UpdateAPIKey(ctx context.Context, record apikey.Key) error {
	if err := validateAPIKey(record); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE auth_api_keys
		SET user_id = ?, name = ?, prefix = ?, token_hash = ?, expires_at = ?, revoked_at = ?
		WHERE api_key_id = ?
	`),
		record.UserID.String(),
		strings.TrimSpace(record.Name),
		strings.TrimSpace(record.Prefix),
		strings.TrimSpace(record.TokenHash),
		nullableTimeString(record.ExpiresAt),
		nullableTimeString(record.RevokedAt),
		record.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	return requireAffected(result, "api key")
}

func (s *sqlStore) queryAPIKey(ctx context.Context, query string, args ...any) (apikey.Key, error) {
	var rawID, rawUserID, name, prefix, tokenHash, createdAt string
	var expiresAt, revokedAt sql.NullString
	err := s.db.QueryRowContext(ctx, s.rebind(query), args...).Scan(
		&rawID,
		&rawUserID,
		&name,
		&prefix,
		&tokenHash,
		&expiresAt,
		&revokedAt,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apikey.Key{}, fmt.Errorf("api key not found")
		}
		return apikey.Key{}, fmt.Errorf("query api key: %w", err)
	}
	return scanAPIKey(rawID, rawUserID, name, prefix, tokenHash, expiresAt, revokedAt, createdAt)
}

func (s *sqlStore) ensureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS auth_passwords (
			user_id TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			session_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			revoked_at TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS auth_api_keys (
			api_key_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT,
			revoked_at TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_user_id ON auth_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_api_keys_user_id ON auth_api_keys(user_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, s.rebind(statement)); err != nil {
			return fmt.Errorf("ensure auth schema: %w", err)
		}
	}
	return nil
}

func (s *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}
