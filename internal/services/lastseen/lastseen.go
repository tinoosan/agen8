// Package lastseen tracks the last time a user viewed a project dashboard.
//
// The marker is stored per (user_id, project_id): one UTC timestamp. Two RPCs
// expose it — get returns the current marker (or zero if never set), and
// mark-seen sets it to now. The web client reads the PREVIOUS marker before
// updating it so the "since you were away" diff spans the right window.
//
// State is durable (SQLite) so the marker survives daemon restarts.
package lastseen

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

// ErrNotFound is returned by Get when no marker exists for the (user, project)
// pair. Callers treat it as a zero time (everything is "new").
var ErrNotFound = errors.New("lastseen: not found")

// Store is the per-(user, project) last-seen marker store. It follows the
// self-migrating vertical pattern: ensureSchema runs at construction.
type Store struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

// NewStore builds the store and provisions the table.
func NewStore(handle *storagedb.Handle) (*Store, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("lastseen: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("lastseen: storage driver must be sqlite, got %q", handle.Driver())
	}
	s := &Store{db: handle.DB(), dialect: handle.Dialect()}
	if err := s.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS user_project_last_seen (
			user_id    TEXT NOT NULL,
			project_id TEXT NOT NULL,
			seen_at    TEXT NOT NULL,
			PRIMARY KEY (user_id, project_id)
		)
	`)
	return err
}

// Get returns the last-seen timestamp for (userID, projectID). Returns
// ErrNotFound when no record exists.
func (s *Store) Get(ctx context.Context, userID, projectID string) (time.Time, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return time.Time{}, fmt.Errorf("lastseen: userID and projectID are required")
	}
	var seenAt string
	err := s.db.QueryRowContext(ctx,
		s.rebind(`SELECT seen_at FROM user_project_last_seen WHERE user_id = ? AND project_id = ?`),
		userID, projectID,
	).Scan(&seenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("lastseen get %s/%s: %w", userID, projectID, err)
	}
	t, err := time.Parse(time.RFC3339Nano, seenAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("lastseen parse %s/%s: %w", userID, projectID, err)
	}
	return t.UTC(), nil
}

// MarkSeen upserts the last-seen timestamp to now for (userID, projectID).
func (s *Store) MarkSeen(ctx context.Context, userID, projectID string, now time.Time) error {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return fmt.Errorf("lastseen: userID and projectID are required")
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO user_project_last_seen (user_id, project_id, seen_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, project_id) DO UPDATE SET seen_at = excluded.seen_at
	`), userID, projectID, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("lastseen mark-seen %s/%s: %w", userID, projectID, err)
	}
	return nil
}

func (s *Store) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}
