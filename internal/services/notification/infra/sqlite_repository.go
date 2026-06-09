package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/services/notification/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

var _ domain.Repository = (*SQLiteRepository)(nil)

// selectColumns is the canonical column list; must match scanNotification order.
const selectColumns = `
	id, user_id, project_id, source, trigger_name, severity,
	subject_kind, subject_id, title, body, link_surface, link_url,
	metadata, throttle_key, created_at, read_at, dismissed_at`

// SQLiteRepository persists notifications to the shared SQLite handle.
type SQLiteRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLiteRepository(handle *storagedb.Handle) *SQLiteRepository {
	return &SQLiteRepository{db: handle.DB(), dialect: handle.Dialect()}
}

func (r *SQLiteRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func (r *SQLiteRepository) Insert(ctx context.Context, n domain.Notification) error {
	if strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("insert notification: id is required")
	}

	metadataJSON := []byte("{}")
	if len(n.Metadata) > 0 {
		b, err := json.Marshal(n.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metadataJSON = b
	}

	_, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO notifications (
			id, user_id, project_id, source, trigger_name, severity,
			subject_kind, subject_id, title, body, link_surface, link_url,
			metadata, throttle_key, created_at, read_at, dismissed_at
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?
		)`),
		strings.TrimSpace(n.ID),
		strings.TrimSpace(n.UserID),
		strings.TrimSpace(n.ProjectID),
		strings.TrimSpace(n.Source),
		strings.TrimSpace(n.Trigger),
		string(n.Severity),
		strings.TrimSpace(n.SubjectKind),
		strings.TrimSpace(n.SubjectID),
		strings.TrimSpace(n.Title),
		n.Body,
		strings.TrimSpace(n.LinkSurface),
		strings.TrimSpace(n.LinkURL),
		string(metadataJSON),
		strings.TrimSpace(n.ThrottleKey),
		n.CreatedAt.UTC().Format(time.RFC3339Nano),
		nullableTime(n.ReadAt),
		nullableTime(n.DismissedAt),
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) ListActive(ctx context.Context, userID, projectID string) ([]domain.Notification, error) {
	query := `SELECT` + selectColumns + `
		FROM notifications
		WHERE user_id = ? AND project_id = ? AND dismissed_at IS NULL
		ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, r.rebind(query), strings.TrimSpace(userID), strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var out []domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return out, nil
}

func (r *SQLiteRepository) ExistsByThrottleKey(ctx context.Context, userID, projectID, throttleKey string) (bool, error) {
	key := strings.TrimSpace(throttleKey)
	if key == "" {
		return false, nil
	}
	var count int
	err := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT COUNT(*) FROM notifications
		WHERE user_id = ? AND project_id = ? AND throttle_key = ?`),
		strings.TrimSpace(userID), strings.TrimSpace(projectID), key,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists by throttle key: %w", err)
	}
	return count > 0, nil
}

func (r *SQLiteRepository) MarkRead(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE notifications SET read_at = ?
		WHERE id = ? AND user_id = ? AND read_at IS NULL`),
		time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(id), strings.TrimSpace(userID),
	)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	_ = res
	return nil
}

func (r *SQLiteRepository) MarkAllRead(ctx context.Context, userID, projectID string) (int, error) {
	res, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE notifications SET read_at = ?
		WHERE user_id = ? AND project_id = ? AND read_at IS NULL AND dismissed_at IS NULL`),
		time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(userID), strings.TrimSpace(projectID),
	)
	if err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark all read rows affected: %w", err)
	}
	return int(n), nil
}

func (r *SQLiteRepository) Dismiss(ctx context.Context, userID, id string) error {
	_, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE notifications SET dismissed_at = ?
		WHERE id = ? AND user_id = ? AND dismissed_at IS NULL`),
		time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(id), strings.TrimSpace(userID),
	)
	if err != nil {
		return fmt.Errorf("dismiss notification: %w", err)
	}
	return nil
}

// -- scan helpers ------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanNotification(s rowScanner) (domain.Notification, error) {
	var (
		n            domain.Notification
		severity     string
		body         sql.NullString
		linkSurface  sql.NullString
		linkURL      sql.NullString
		metadataJSON sql.NullString
		throttleKey  sql.NullString
		createdAt    string
		readAt       sql.NullString
		dismissedAt  sql.NullString
	)

	err := s.Scan(
		&n.ID, &n.UserID, &n.ProjectID, &n.Source, &n.Trigger, &severity,
		&n.SubjectKind, &n.SubjectID, &n.Title, &body, &linkSurface, &linkURL,
		&metadataJSON, &throttleKey, &createdAt, &readAt, &dismissedAt,
	)
	if err != nil {
		return domain.Notification{}, fmt.Errorf("scan notification: %w", err)
	}

	n.Severity = domain.Severity(strings.TrimSpace(severity))
	n.Body = body.String
	n.LinkSurface = linkSurface.String
	n.LinkURL = linkURL.String
	n.ThrottleKey = throttleKey.String

	created, perr := time.Parse(time.RFC3339Nano, createdAt)
	if perr != nil {
		return domain.Notification{}, fmt.Errorf("parse created_at %q: %w", createdAt, perr)
	}
	n.CreatedAt = created

	if t, ok := parseNullableTime(readAt); ok {
		n.ReadAt = t
	}
	if t, ok := parseNullableTime(dismissedAt); ok {
		n.DismissedAt = t
	}

	if v := strings.TrimSpace(metadataJSON.String); v != "" && v != "{}" {
		m := make(map[string]string)
		if jerr := json.Unmarshal([]byte(v), &m); jerr != nil {
			return domain.Notification{}, fmt.Errorf("parse metadata: %w", jerr)
		}
		n.Metadata = m
	}

	return n, nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseNullableTime(v sql.NullString) (*time.Time, bool) {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil, false
	}
	t, err := time.Parse(time.RFC3339Nano, v.String)
	if err != nil {
		return nil, false
	}
	return &t, true
}
