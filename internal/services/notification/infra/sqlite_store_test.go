package infra

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, `
				CREATE TABLE notifications (
					id           TEXT PRIMARY KEY,
					user_id      TEXT NOT NULL,
					project_id   TEXT NOT NULL DEFAULT '',
					source       TEXT NOT NULL,
					trigger_name TEXT NOT NULL,
					severity     TEXT NOT NULL,
					subject_kind TEXT NOT NULL DEFAULT '',
					subject_id   TEXT NOT NULL DEFAULT '',
					title        TEXT NOT NULL,
					body         TEXT,
					link_surface TEXT,
					link_url     TEXT,
					metadata     TEXT,
					throttle_key TEXT,
					created_at   DATETIME NOT NULL,
					read_at      DATETIME,
					dismissed_at DATETIME
				);
				CREATE INDEX idx_notifications_user_unread
					ON notifications(user_id, read_at) WHERE dismissed_at IS NULL;
				CREATE INDEX idx_notifications_throttle
					ON notifications(user_id, source, trigger_name, throttle_key, created_at DESC);
				CREATE INDEX idx_notifications_subject
					ON notifications(user_id, source, subject_kind, subject_id) WHERE dismissed_at IS NULL;

				CREATE TABLE notification_rules (
					id               TEXT PRIMARY KEY,
					user_id          TEXT NOT NULL,
					source           TEXT NOT NULL,
					trigger_name     TEXT NOT NULL,
					min_severity     TEXT NOT NULL DEFAULT 'info',
					channels         TEXT NOT NULL,
					cooldown_minutes INTEGER NOT NULL DEFAULT 30,
					enabled          BOOLEAN NOT NULL DEFAULT 1,
					webhook_url      TEXT
				);
				CREATE INDEX idx_notification_rules_user
					ON notification_rules(user_id, enabled);
			`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func TestSQLiteNotificationStore_SaveAndFind(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	n := domain.Notification{
		ID:          uuid.NewString(),
		UserID:      "prof-1",
		Source:      "heartbeat",
		Trigger:     "outcome_critical",
		Severity:    domain.SeverityCritical,
		Title:       "Heartbeat critical: check_api",
		Body:        "API returned 500",
		Link:        &domain.NotificationLink{Surface: "calendar", URL: "/calendar?task=123"},
		ThrottleKey: "check_api",
		Metadata:    map[string]string{"jobName": "check_api", "taskId": "task-1"},
		CreatedAt:   time.Now(),
	}

	if err := store.Save(ctx, n); err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := store.FindByUser(ctx, "prof-1", domain.NotificationFilter{})
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	got := results[0]
	if got.ID != n.ID {
		t.Errorf("ID mismatch: %q != %q", got.ID, n.ID)
	}
	if got.Source != "heartbeat" {
		t.Errorf("source mismatch: %q", got.Source)
	}
	if got.Link == nil || got.Link.Surface != "calendar" {
		t.Errorf("link mismatch: %+v", got.Link)
	}
	if got.Metadata["jobName"] != "check_api" {
		t.Errorf("metadata mismatch: %v", got.Metadata)
	}
}

func TestSQLiteNotificationStore_UnreadCount(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	// Save 3 notifications, mark 1 as read
	for i := 0; i < 3; i++ {
		n := domain.Notification{
			ID: uuid.NewString(), UserID: "prof-1", Source: "test",
			Trigger: "test", Severity: domain.SeverityInfo, Title: "test",
			CreatedAt: time.Now(),
		}
		if err := store.Save(ctx, n); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if i == 0 {
			if err := store.MarkRead(ctx, n.ID); err != nil {
				t.Fatalf("MarkRead: %v", err)
			}
		}
	}

	count, err := store.UnreadCount(ctx, "prof-1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread, got %d", count)
	}
}

func TestSQLiteNotificationStore_MarkAllRead(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		n := domain.Notification{
			ID: uuid.NewString(), UserID: "prof-1", Source: "test",
			Trigger: "test", Severity: domain.SeverityInfo, Title: "test",
			CreatedAt: time.Now(),
		}
		store.Save(ctx, n)
	}

	if err := store.MarkAllRead(ctx, "prof-1"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}

	count, err := store.UnreadCount(ctx, "prof-1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread after MarkAllRead, got %d", count)
	}
}

func TestSQLiteNotificationStore_Dismiss(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	n := domain.Notification{
		ID: uuid.NewString(), UserID: "prof-1", Source: "test",
		Trigger: "test", Severity: domain.SeverityInfo, Title: "test",
		CreatedAt: time.Now(),
	}
	store.Save(ctx, n)
	store.Dismiss(ctx, n.ID)

	// Dismissed notifications should not appear in FindByUser
	results, err := store.FindByUser(ctx, "prof-1", domain.NotificationFilter{})
	if err != nil {
		t.Fatalf("FindByUser after dismiss: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after dismiss, got %d", len(results))
	}
}

func TestSQLiteNotificationStore_LastByThrottleKey(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	// Save two notifications with the same throttle key
	n1 := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, Title: "first", ThrottleKey: "check_api",
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	n2 := domain.Notification{
		ID: "n2", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, Title: "second", ThrottleKey: "check_api",
		CreatedAt: time.Now(),
	}
	store.Save(ctx, n1)
	store.Save(ctx, n2)

	last, err := store.LastByThrottleKey(ctx, "prof-1", "heartbeat", "outcome_critical", "check_api")
	if err != nil {
		t.Fatalf("LastByThrottleKey: %v", err)
	}
	if last == nil || last.ID != "n2" {
		t.Errorf("expected last to be n2, got %v", last)
	}
}

func TestSQLiteNotificationStore_FindByUser_Filters(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	// Mix of sources and severities
	notifications := []domain.Notification{
		{ID: "1", UserID: "prof-1", Source: "heartbeat", Trigger: "t", Severity: domain.SeverityCritical, Title: "a", CreatedAt: time.Now()},
		{ID: "2", UserID: "prof-1", Source: "heartbeat", Trigger: "t", Severity: domain.SeverityWarning, Title: "b", CreatedAt: time.Now()},
		{ID: "3", UserID: "prof-1", Source: "task", Trigger: "t", Severity: domain.SeverityInfo, Title: "c", CreatedAt: time.Now()},
	}
	for _, n := range notifications {
		store.Save(ctx, n)
	}

	// Filter by source
	results, err := store.FindByUser(ctx, "prof-1", domain.NotificationFilter{Source: "heartbeat"})
	if err != nil {
		t.Fatalf("FindByUser source filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("source filter: expected 2, got %d", len(results))
	}

	// Filter by severity
	results, err = store.FindByUser(ctx, "prof-1", domain.NotificationFilter{Severity: domain.SeverityWarning})
	if err != nil {
		t.Fatalf("FindByUser severity filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("severity filter: expected 2 (warning+critical), got %d", len(results))
	}

	// Filter unread only
	store.MarkRead(ctx, "1")
	unread := true
	results, err = store.FindByUser(ctx, "prof-1", domain.NotificationFilter{Unread: &unread})
	if err != nil {
		t.Fatalf("FindByUser unread filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("unread filter: expected 2, got %d", len(results))
	}

	// Limit
	results, err = store.FindByUser(ctx, "prof-1", domain.NotificationFilter{Limit: 1})
	if err != nil {
		t.Fatalf("FindByUser limit filter: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("limit filter: expected 1, got %d", len(results))
	}
}

func TestSQLiteNotificationRuleStore_CRUD(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationRuleStore(handle)
	ctx := context.Background()

	rule := domain.NotificationRule{
		ID:              uuid.NewString(),
		UserID:          "prof-1",
		Source:          "heartbeat",
		Trigger:         "outcome_critical",
		MinSeverity:     domain.SeverityCritical,
		Channels:        []string{"in_app", "webhook"},
		CooldownMinutes: 30,
		Enabled:         true,
	}

	// Save
	if err := store.Save(ctx, rule); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// FindByUser
	rules, err := store.FindByUser(ctx, "prof-1")
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Channels[0] != "in_app" || rules[0].Channels[1] != "webhook" {
		t.Errorf("channels mismatch: %v", rules[0].Channels)
	}

	// FindMatching
	matched, err := store.FindMatching(ctx, "prof-1", "heartbeat", "outcome_critical", domain.SeverityCritical)
	if err != nil {
		t.Fatalf("FindMatching: %v", err)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}

	// FindMatching with lower severity — should not match
	matched, err = store.FindMatching(ctx, "prof-1", "heartbeat", "outcome_critical", domain.SeverityInfo)
	if err != nil {
		t.Fatalf("FindMatching info severity: %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for info severity, got %d", len(matched))
	}

	// Update (upsert)
	rule.CooldownMinutes = 60
	if err := store.Save(ctx, rule); err != nil {
		t.Fatalf("Save updated rule: %v", err)
	}
	rules, err = store.FindByUser(ctx, "prof-1")
	if err != nil {
		t.Fatalf("FindByUser after update: %v", err)
	}
	if rules[0].CooldownMinutes != 60 {
		t.Errorf("expected cooldown 60, got %d", rules[0].CooldownMinutes)
	}

	// Delete
	if err := store.Delete(ctx, rule.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rules, err = store.FindByUser(ctx, "prof-1")
	if err != nil {
		t.Fatalf("FindByUser after delete: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestSQLiteNotificationRuleStore_WildcardMatching(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationRuleStore(handle)
	ctx := context.Background()

	// Wildcard rule: any source, any trigger, critical
	rule := domain.NotificationRule{
		ID: uuid.NewString(), UserID: "prof-1",
		Source: "*", Trigger: "*", MinSeverity: domain.SeverityCritical,
		Channels: []string{"in_app"}, CooldownMinutes: 30, Enabled: true,
	}
	if err := store.Save(ctx, rule); err != nil {
		t.Fatalf("Save wildcard rule: %v", err)
	}

	// Should match any critical notification
	matched, err := store.FindMatching(ctx, "prof-1", "task", "task_failed", domain.SeverityCritical)
	if err != nil {
		t.Fatalf("FindMatching critical: %v", err)
	}
	if len(matched) != 1 {
		t.Errorf("expected 1 wildcard match, got %d", len(matched))
	}

	// Should not match warning
	matched, err = store.FindMatching(ctx, "prof-1", "task", "task_failed", domain.SeverityWarning)
	if err != nil {
		t.Fatalf("FindMatching warning: %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for warning, got %d", len(matched))
	}
}

func TestSQLiteNotificationStore_Prune(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationStore(handle)
	ctx := context.Background()

	// Create old notification (35 days ago) — should be pruned by age
	old := domain.Notification{
		ID: "old", UserID: "prof-1", Source: "test", Trigger: "t",
		Severity: domain.SeverityInfo, Title: "old",
		CreatedAt: time.Now().Add(-35 * 24 * time.Hour),
	}
	store.Save(ctx, old)

	// Create dismissed notification — dismissed 8 days ago, should be pruned
	dismissed := domain.Notification{
		ID: "dismissed", UserID: "prof-1", Source: "test", Trigger: "t",
		Severity: domain.SeverityInfo, Title: "dismissed",
		CreatedAt: time.Now().Add(-10 * 24 * time.Hour),
	}
	store.Save(ctx, dismissed)
	// Directly set dismissed_at to 8 days ago (store.Dismiss sets it to now)
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := handle.DB().ExecContext(ctx, `UPDATE notifications SET dismissed_at = ? WHERE id = 'dismissed'`, eightDaysAgo); err != nil {
		t.Fatalf("set dismissed_at: %v", err)
	}

	// Create recent notification — should survive
	recent := domain.Notification{
		ID: "recent", UserID: "prof-1", Source: "test", Trigger: "t",
		Severity: domain.SeverityInfo, Title: "recent",
		CreatedAt: time.Now(),
	}
	store.Save(ctx, recent)

	pruned, err := store.Prune(ctx, domain.RetentionPolicy{MaxAgeDays: 30, MaxPerUser: 1000})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	// Only the recent notification should remain
	var count int
	if err := handle.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications`).Scan(&count); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 remaining notification, got %d", count)
	}
}

func TestSQLiteNotificationRuleStore_Delete_NotFound(t *testing.T) {
	handle := setupTestDB(t)
	store := NewSQLiteNotificationRuleStore(handle)
	ctx := context.Background()

	if err := store.Delete(ctx, "nonexistent-id"); err == nil {
		t.Fatal("expected error for Delete on nonexistent rule, got nil")
	}
}
