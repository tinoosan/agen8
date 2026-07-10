package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
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

func (s *sqlStore) Get(ctx context.Context, id user.ID) (user.User, error) {
	if id.IsZero() {
		return user.User{}, fmt.Errorf("user id is required")
	}
	return s.queryOne(ctx, `
		SELECT user_id, email, name, role, lifecycle, preferences_json, created_at, updated_at
		FROM users
		WHERE user_id = ?
	`, id.String())
}

func (s *sqlStore) GetByEmail(ctx context.Context, email string) (user.User, error) {
	email = normalizeEmail(email)
	if email == "" {
		return user.User{}, fmt.Errorf("user email is required")
	}
	return s.queryOne(ctx, `
		SELECT user_id, email, name, role, lifecycle, preferences_json, created_at, updated_at
		FROM users
		WHERE email = ?
	`, email)
}

func (s *sqlStore) FirstActive(ctx context.Context) (user.User, error) {
	return s.queryOne(ctx, `
		SELECT user_id, email, name, role, lifecycle, preferences_json, created_at, updated_at
		FROM users
		WHERE lifecycle = ?
		ORDER BY created_at ASC, user_id ASC
		LIMIT 1
	`, string(user.LifecycleActive))
}

func (s *sqlStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *sqlStore) Create(ctx context.Context, record user.User) error {
	if err := validateUserRecord(record); err != nil {
		return err
	}
	preferences, err := preferencesString(record.Preferences)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO users (
			user_id, email, name, role, lifecycle, preferences_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`),
		record.ID.String(),
		normalizeEmail(record.Email),
		strings.TrimSpace(record.Name),
		string(record.Role),
		string(record.Lifecycle),
		preferences,
		timeString(record.CreatedAt),
		timeString(record.UpdatedAt),
	); err != nil {
		return fmt.Errorf("create user %s: %w", record.ID.String(), mapEmailUniqueError(err))
	}
	return nil
}

func (s *sqlStore) Update(ctx context.Context, record user.User) error {
	if err := validateUserRecord(record); err != nil {
		return err
	}
	preferences, err := preferencesString(record.Preferences)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE users
		SET email = ?, name = ?, role = ?, lifecycle = ?, preferences_json = ?, updated_at = ?
		WHERE user_id = ?
	`),
		normalizeEmail(record.Email),
		strings.TrimSpace(record.Name),
		string(record.Role),
		string(record.Lifecycle),
		preferences,
		timeString(record.UpdatedAt),
		record.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update user %s: %w", record.ID.String(), mapEmailUniqueError(err))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user %s: rows affected: %w", record.ID.String(), err)
	}
	if affected == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (s *sqlStore) queryOne(ctx context.Context, query string, args ...any) (user.User, error) {
	var (
		rawID     string
		email     string
		name      string
		role      string
		lifecycle string
		rawPrefs  string
		createdAt string
		updatedAt string
	)
	err := s.db.QueryRowContext(ctx, s.rebind(query), args...).Scan(
		&rawID,
		&email,
		&name,
		&role,
		&lifecycle,
		&rawPrefs,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user.User{}, user.ErrNotFound
		}
		return user.User{}, fmt.Errorf("query user: %w", err)
	}
	id, err := user.NewID(rawID)
	if err != nil {
		return user.User{}, fmt.Errorf("scan user id: %w", err)
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return user.User{}, fmt.Errorf("scan user created at: %w", err)
	}
	updated, err := parseTime(updatedAt)
	if err != nil {
		return user.User{}, fmt.Errorf("scan user updated at: %w", err)
	}
	preferences, err := parsePreferences(rawPrefs)
	if err != nil {
		return user.User{}, fmt.Errorf("scan user preferences: %w", err)
	}
	record := user.User{
		ID:          id,
		Email:       email,
		Name:        name,
		Role:        user.Role(role),
		Lifecycle:   user.Lifecycle(lifecycle),
		Preferences: preferences,
		CreatedAt:   created,
		UpdatedAt:   updated,
	}
	if err := validateUserRecord(record); err != nil {
		return user.User{}, fmt.Errorf("scan user record: %w", err)
	}
	return record, nil
}

func (s *sqlStore) ensureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, s.rebind(`
		CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			role TEXT NOT NULL,
			lifecycle TEXT NOT NULL,
			preferences_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)); err != nil {
		return fmt.Errorf("ensure users table: %w", err)
	}
	if err := s.ensurePreferencesColumn(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email)`); err != nil {
		return fmt.Errorf("ensure users email index: %w", err)
	}
	return nil
}

func (s *sqlStore) ensurePreferencesColumn(ctx context.Context) (returnErr error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(users)`)
	if err != nil {
		return fmt.Errorf("ensure users preferences column: table info: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("ensure users preferences column: close table info: %w", err)
		}
	}()
	hasColumn := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("ensure users preferences column: scan table info: %w", err)
		}
		if strings.TrimSpace(name) == "preferences_json" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ensure users preferences column: iterate table info: %w", err)
	}
	if hasColumn {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN preferences_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return fmt.Errorf("ensure users preferences column: add column: %w", err)
	}
	return nil
}

func (s *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}
