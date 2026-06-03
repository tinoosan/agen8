// Package infra provides infrastructure implementations for the notification bounded context.
package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

// SQLiteNotificationStore implements domain.NotificationRepository backed by SQLite.
type SQLiteNotificationStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

var _ domain.NotificationRepository = (*SQLiteNotificationStore)(nil)

// NewSQLiteNotificationStore creates a new SQLite-backed notification repository.
func NewSQLiteNotificationStore(handle *storagedb.Handle) *SQLiteNotificationStore {
	return &SQLiteNotificationStore{db: handle.DB(), dialect: handle.Dialect()}
}

func (s *SQLiteNotificationStore) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}

// notificationColumns lists every column read by scanNotifications, in
// the same order. Centralized so SELECT lists stay in sync with the
// scanner without hand-counting placeholders at every call site.
const notificationColumns = `id, user_id, project_id, source, trigger_name, severity,
		subject_kind, subject_id, title, body,
		link_surface, link_url, metadata, throttle_key,
		created_at, read_at, dismissed_at`

// Save persists a notification to the notifications table.
func (s *SQLiteNotificationStore) Save(ctx context.Context, n domain.Notification) error {
	metadataJSON := ""
	if len(n.Metadata) > 0 {
		b, err := json.Marshal(n.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metadataJSON = string(b)
	}

	var linkSurface, linkURL *string
	if n.Link != nil {
		linkSurface = &n.Link.Surface
		linkURL = &n.Link.URL
	}

	_, err := s.db.ExecContext(ctx, s.rebind(
		`INSERT INTO notifications (id, user_id, project_id, source, trigger_name, severity,
			subject_kind, subject_id, title, body,
			link_surface, link_url, metadata, throttle_key,
			created_at, read_at, dismissed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	),
		n.ID, n.UserID, n.ProjectID, n.Source, n.Trigger, string(n.Severity),
		n.Subject.Kind, n.Subject.ID,
		n.Title, n.Body, linkSurface, linkURL,
		nullIfEmpty(metadataJSON), nullIfEmpty(n.ThrottleKey),
		n.CreatedAt.UTC().Format(time.RFC3339Nano),
		formatTimePtr(n.ReadAt), formatTimePtr(n.DismissedAt),
	)
	if err != nil {
		return fmt.Errorf("save notification: %w", err)
	}
	return nil
}

// FindByUser returns notifications for a user, applying the given filter.
// Results are ordered by created_at DESC (newest first).
func (s *SQLiteNotificationStore) FindByUser(ctx context.Context, userID string, f domain.NotificationFilter) ([]domain.Notification, error) {
	query := `SELECT ` + notificationColumns + `
		FROM notifications WHERE user_id = ? AND dismissed_at IS NULL`
	args := []any{userID}

	if f.ProjectID != "" {
		query += " AND project_id = ?"
		args = append(args, f.ProjectID)
	}
	if f.Source != "" {
		query += " AND source = ?"
		args = append(args, f.Source)
	}
	if f.Severity != "" {
		query += " AND severity IN (" + severityInClause(f.Severity) + ")"
	}
	if f.Unread != nil {
		if *f.Unread {
			query += " AND read_at IS NULL"
		} else {
			query += " AND read_at IS NOT NULL"
		}
	}

	query += " ORDER BY created_at DESC"

	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("find notifications: %w", err)
	}
	defer rows.Close()

	return scanNotifications(rows)
}

// MarkRead sets the read_at timestamp for a notification.
func (s *SQLiteNotificationStore) MarkRead(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE notifications SET read_at = ? WHERE id = ?`,
	),
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// MarkAllRead sets the read_at timestamp for all unread notifications for a user.
func (s *SQLiteNotificationStore) MarkAllRead(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`,
	),
		time.Now().UTC().Format(time.RFC3339Nano), userID,
	)
	return err
}

// Dismiss sets the dismissed_at timestamp for a notification.
func (s *SQLiteNotificationStore) Dismiss(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE notifications SET dismissed_at = ? WHERE id = ?`,
	),
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// DismissBySubject auto-dismisses every active notification matching
// (userID, source, subject). Returns rows affected. The empty subject
// rejected — caller must pass a fully-populated Subject so we don't
// accidentally bulk-dismiss every "subject-less" notification (e.g.
// heartbeat summaries) the user has.
func (s *SQLiteNotificationStore) DismissBySubject(ctx context.Context, userID, source string, subject domain.Subject) (int, error) {
	if subject.IsZero() {
		return 0, fmt.Errorf("dismiss by subject: subject is required")
	}
	if strings.TrimSpace(userID) == "" {
		return 0, fmt.Errorf("dismiss by subject: userId is required")
	}
	if strings.TrimSpace(source) == "" {
		return 0, fmt.Errorf("dismiss by subject: source is required")
	}
	res, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE notifications SET dismissed_at = ?
		 WHERE user_id = ? AND source = ? AND subject_kind = ? AND subject_id = ?
		   AND dismissed_at IS NULL`,
	),
		time.Now().UTC().Format(time.RFC3339Nano),
		userID, source, subject.Kind, subject.ID,
	)
	if err != nil {
		return 0, fmt.Errorf("dismiss by subject: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("dismiss by subject rows affected: %w", err)
	}
	return int(n), nil
}

// UnreadCount returns the number of unread, non-dismissed notifications for a user.
func (s *SQLiteNotificationStore) UnreadCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, s.rebind(
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL AND dismissed_at IS NULL`,
	),
		userID,
	).Scan(&count)
	return count, err
}

// LastByThrottleKey returns the most recent notification matching the throttle composite key.
// Returns (nil, nil) if no match exists.
func (s *SQLiteNotificationStore) LastByThrottleKey(ctx context.Context, userID, source, trigger, throttleKey string) (*domain.Notification, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT `+notificationColumns+`
		 FROM notifications
		 WHERE user_id = ? AND source = ? AND trigger_name = ? AND throttle_key = ?
		 ORDER BY created_at DESC LIMIT 1`,
	),
		userID, source, trigger, throttleKey,
	)
	if err != nil {
		return nil, fmt.Errorf("last by throttle key: %w", err)
	}
	defer rows.Close()

	notifications, err := scanNotifications(rows)
	if err != nil {
		return nil, err
	}
	if len(notifications) == 0 {
		return nil, nil
	}
	return &notifications[0], nil
}

// Prune removes notifications according to the retention policy.
func (s *SQLiteNotificationStore) Prune(ctx context.Context, policy domain.RetentionPolicy) (int, error) {
	total := 0

	// 1. Dismissed notifications older than 7 days
	res, err := s.db.ExecContext(ctx, s.rebind(
		`DELETE FROM notifications WHERE dismissed_at IS NOT NULL AND dismissed_at < ?`,
	),
		time.Now().Add(-7*24*time.Hour).UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("prune dismissed: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, fmt.Errorf("prune dismissed rows affected: %w", err)
	} else if n > 0 {
		total += int(n)
	}

	// 2. Age-based pruning
	if policy.MaxAgeDays > 0 {
		cutoff := time.Now().Add(-time.Duration(policy.MaxAgeDays) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		res, err = s.db.ExecContext(ctx, s.rebind(
			`DELETE FROM notifications WHERE created_at < ?`), cutoff,
		)
		if err != nil {
			return total, fmt.Errorf("prune by age: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return total, fmt.Errorf("prune by age rows affected: %w", err)
		} else if n > 0 {
			total += int(n)
		}
	}

	// 3. Count-based pruning per user (keep newest MaxPerUser)
	if policy.MaxPerUser > 0 {
		userRows, err := s.db.QueryContext(ctx,
			s.rebind(`SELECT DISTINCT user_id FROM notifications`))
		if err != nil {
			return total, fmt.Errorf("prune count: list users: %w", err)
		}
		var userIDs []string
		for userRows.Next() {
			var uid string
			if err := userRows.Scan(&uid); err != nil {
				userRows.Close()
				return total, err
			}
			userIDs = append(userIDs, uid)
		}
		userRows.Close()

		for _, uid := range userIDs {
			res, err = s.db.ExecContext(ctx, s.rebind(
				`DELETE FROM notifications WHERE user_id = ? AND id NOT IN (
					SELECT id FROM notifications WHERE user_id = ?
					ORDER BY created_at DESC LIMIT ?
				)`), uid, uid, policy.MaxPerUser,
			)
			if err != nil {
				return total, fmt.Errorf("prune count for user %s: %w", uid, err)
			}
			if n, err := res.RowsAffected(); err != nil {
				return total, fmt.Errorf("prune count for user %s rows affected: %w", uid, err)
			} else if n > 0 {
				total += int(n)
			}
		}
	}

	return total, nil
}

// ── SQLiteNotificationRuleStore ─────────────────────────────────────────

// SQLiteNotificationRuleStore implements domain.NotificationRuleRepository backed by SQLite.
type SQLiteNotificationRuleStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

var _ domain.NotificationRuleRepository = (*SQLiteNotificationRuleStore)(nil)

// NewSQLiteNotificationRuleStore creates a new SQLite-backed rule repository.
func NewSQLiteNotificationRuleStore(handle *storagedb.Handle) *SQLiteNotificationRuleStore {
	return &SQLiteNotificationRuleStore{db: handle.DB(), dialect: handle.Dialect()}
}

func (s *SQLiteNotificationRuleStore) rebind(query string) string {
	return storagedb.Rebind(query, s.dialect)
}

const ruleColumns = `id, user_id, source, trigger_name, min_severity, channels, cooldown_minutes, enabled, webhook_url`

// FindByUser returns all rules for a user.
func (s *SQLiteNotificationRuleStore) FindByUser(ctx context.Context, userID string) ([]domain.NotificationRule, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT `+ruleColumns+`
		 FROM notification_rules WHERE user_id = ?
		 ORDER BY source, trigger_name`), userID,
	)
	if err != nil {
		return nil, fmt.Errorf("find rules: %w", err)
	}
	defer rows.Close()

	return scanRules(rows)
}

// FindMatching returns rules that match the given notification parameters.
// A rule matches if: source matches (or wildcard), trigger matches (or wildcard),
// severity >= minSeverity, and the rule is enabled.
func (s *SQLiteNotificationRuleStore) FindMatching(ctx context.Context, userID, source, trigger string, severity domain.Severity) ([]domain.NotificationRule, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(
		`SELECT `+ruleColumns+`
		 FROM notification_rules
		 WHERE user_id = ? AND enabled = 1
		   AND (source = ? OR source = '*')
		   AND (trigger_name = ? OR trigger_name = '*')`,
	),
		userID, source, trigger,
	)
	if err != nil {
		return nil, fmt.Errorf("find matching rules: %w", err)
	}
	defer rows.Close()

	all, err := scanRules(rows)
	if err != nil {
		return nil, err
	}

	var matched []domain.NotificationRule
	for _, r := range all {
		if domain.SeverityRank(severity) >= domain.SeverityRank(r.MinSeverity) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

// Save inserts or updates a notification rule (upsert on ID).
func (s *SQLiteNotificationRuleStore) Save(ctx context.Context, rule domain.NotificationRule) error {
	channelsJSON, err := json.Marshal(rule.Channels)
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}

	_, err = s.db.ExecContext(ctx, s.rebind(
		`INSERT INTO notification_rules (id, user_id, source, trigger_name, min_severity, channels, cooldown_minutes, enabled, webhook_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			source = excluded.source,
			trigger_name = excluded.trigger_name,
			min_severity = excluded.min_severity,
			channels = excluded.channels,
			cooldown_minutes = excluded.cooldown_minutes,
			enabled = excluded.enabled,
			webhook_url = excluded.webhook_url`,
	),
		rule.ID, rule.UserID, rule.Source, rule.Trigger,
		string(rule.MinSeverity), string(channelsJSON),
		rule.CooldownMinutes, rule.Enabled, nullIfEmpty(rule.WebhookURL),
	)
	if err != nil {
		return fmt.Errorf("save rule: %w", err)
	}
	return nil
}

// Delete removes a notification rule by ID.
func (s *SQLiteNotificationRuleStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM notification_rules WHERE id = ?`), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete notification rule rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("notification rule not found: %s", id)
	}
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────────

func scanNotifications(rows *sql.Rows) ([]domain.Notification, error) {
	var result []domain.Notification
	for rows.Next() {
		var n domain.Notification
		var severity, createdAt string
		var projectID, subjectKind, subjectID sql.NullString
		var body, linkSurface, linkURL, metadataJSON, throttleKey, readAt, dismissedAt sql.NullString

		if err := rows.Scan(
			&n.ID, &n.UserID, &projectID, &n.Source, &n.Trigger, &severity,
			&subjectKind, &subjectID,
			&n.Title, &body, &linkSurface, &linkURL,
			&metadataJSON, &throttleKey, &createdAt, &readAt, &dismissedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}

		n.Severity = domain.Severity(severity)
		n.ProjectID = projectID.String
		n.Subject = domain.Subject{Kind: subjectKind.String, ID: subjectID.String}
		n.Body = body.String
		n.ThrottleKey = throttleKey.String

		if linkSurface.Valid && linkURL.Valid {
			n.Link = &domain.NotificationLink{Surface: linkSurface.String, URL: linkURL.String}
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &n.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		var err error
		n.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		n.ReadAt = parseTimePtr(readAt)
		n.DismissedAt = parseTimePtr(dismissedAt)

		result = append(result, n)
	}
	return result, rows.Err()
}

func scanRules(rows *sql.Rows) ([]domain.NotificationRule, error) {
	var result []domain.NotificationRule
	for rows.Next() {
		var r domain.NotificationRule
		var minSeverity, channelsJSON string
		var webhookURL sql.NullString

		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Source, &r.Trigger, &minSeverity,
			&channelsJSON, &r.CooldownMinutes, &r.Enabled, &webhookURL,
		); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}

		r.MinSeverity = domain.Severity(minSeverity)
		r.WebhookURL = webhookURL.String
		if err := json.Unmarshal([]byte(channelsJSON), &r.Channels); err != nil {
			return nil, fmt.Errorf("unmarshal channels: %w", err)
		}

		result = append(result, r)
	}
	return result, rows.Err()
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// severityInClause returns a SQL IN clause for severities at or above the given level.
func severityInClause(minSeverity domain.Severity) string {
	switch minSeverity {
	case domain.SeverityCritical:
		return "'critical'"
	case domain.SeverityWarning:
		return "'warning', 'critical'"
	default:
		return "'info', 'warning', 'critical'"
	}
}

// BulkInsertDefaultRules is a helper for seeding default rules at user creation time.
// It uses the SQLiteNotificationRuleStore to save each default rule.
func BulkInsertDefaultRules(ctx context.Context, store domain.NotificationRuleRepository, userID string) error {
	existing, err := store.FindByUser(ctx, userID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	for _, rule := range domain.DefaultRules(userID) {
		if strings.TrimSpace(rule.ID) == "" {
			rule.ID = fmt.Sprintf("default-%s-%s-%s", userID, rule.Source, rule.Trigger)
		}
		if err := store.Save(ctx, rule); err != nil {
			return err
		}
	}
	return nil
}
