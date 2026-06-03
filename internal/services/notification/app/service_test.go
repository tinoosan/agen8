package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// ── Test doubles ────────────────────────────────────────────────────────

type memNotificationStore struct {
	mu            sync.Mutex
	notifications []domain.Notification
}

func (m *memNotificationStore) Save(_ context.Context, n domain.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, n)
	return nil
}

func (m *memNotificationStore) FindByUser(_ context.Context, userID string, f domain.NotificationFilter) ([]domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []domain.Notification
	for _, n := range m.notifications {
		if n.UserID != userID || n.IsDismissed() {
			continue
		}
		if f.Source != "" && n.Source != f.Source {
			continue
		}
		if f.Severity != "" && domain.SeverityRank(n.Severity) < domain.SeverityRank(f.Severity) {
			continue
		}
		if f.Unread != nil {
			if *f.Unread && !n.IsUnread() {
				continue
			}
			if !*f.Unread && n.IsUnread() {
				continue
			}
		}
		result = append(result, n)
	}
	if f.Offset > 0 && f.Offset < len(result) {
		result = result[f.Offset:]
	}
	if f.Limit > 0 && f.Limit < len(result) {
		result = result[:f.Limit]
	}
	return result, nil
}

func (m *memNotificationStore) MarkRead(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for i := range m.notifications {
		if m.notifications[i].ID == id {
			m.notifications[i].ReadAt = &now
			return nil
		}
	}
	return nil
}

func (m *memNotificationStore) MarkAllRead(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for i := range m.notifications {
		if m.notifications[i].UserID == userID && m.notifications[i].ReadAt == nil {
			m.notifications[i].ReadAt = &now
		}
	}
	return nil
}

func (m *memNotificationStore) Dismiss(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for i := range m.notifications {
		if m.notifications[i].ID == id {
			m.notifications[i].DismissedAt = &now
			return nil
		}
	}
	return nil
}

func (m *memNotificationStore) UnreadCount(_ context.Context, userID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifications {
		if n.UserID == userID && n.IsUnread() && !n.IsDismissed() {
			count++
		}
	}
	return count, nil
}

func (m *memNotificationStore) LastByThrottleKey(_ context.Context, userID, source, trigger, throttleKey string) (*domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var latest *domain.Notification
	for i := range m.notifications {
		n := &m.notifications[i]
		if n.UserID == userID && n.Source == source && n.Trigger == trigger && n.ThrottleKey == throttleKey {
			if latest == nil || n.CreatedAt.After(latest.CreatedAt) {
				latest = n
			}
		}
	}
	return latest, nil
}

func (m *memNotificationStore) DismissBySubject(_ context.Context, userID, source string, subject domain.Subject) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	count := 0
	for i := range m.notifications {
		n := &m.notifications[i]
		if n.UserID != userID || n.Source != source {
			continue
		}
		if n.Subject != subject {
			continue
		}
		if n.DismissedAt != nil {
			continue
		}
		n.DismissedAt = &now
		count++
	}
	return count, nil
}

func (m *memNotificationStore) Prune(_ context.Context, _ domain.RetentionPolicy) (int, error) {
	return 0, nil
}

type memRuleStore struct {
	mu    sync.Mutex
	rules []domain.NotificationRule
}

func (m *memRuleStore) FindByUser(_ context.Context, userID string) ([]domain.NotificationRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []domain.NotificationRule
	for _, r := range m.rules {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *memRuleStore) FindMatching(_ context.Context, userID, source, trigger string, severity domain.Severity) ([]domain.NotificationRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []domain.NotificationRule
	for _, r := range m.rules {
		if r.UserID == userID && r.Matches(source, trigger, severity) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *memRuleStore) Save(_ context.Context, rule domain.NotificationRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.rules {
		if r.ID == rule.ID {
			m.rules[i] = rule
			return nil
		}
	}
	m.rules = append(m.rules, rule)
	return nil
}

func (m *memRuleStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, r := range m.rules {
		if r.ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

// stubEvaluator is a TriggerEvaluator that returns configured notifications for any event.
type stubEvaluator struct {
	source string
	result []domain.Notification
}

func (e *stubEvaluator) Source() string { return e.source }

func (e *stubEvaluator) Evaluate(_ context.Context, _ types.EventRecord) []domain.Notification {
	return e.result
}

// stubChannel records all sent notifications.
type stubChannel struct {
	mu       sync.Mutex
	chanType string
	sent     []domain.Notification
}

func (c *stubChannel) Type() string { return c.chanType }

func (c *stubChannel) Send(_ context.Context, n domain.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, n)
	return nil
}

// failingChannel returns a configured error on Send and records the attempts.
type failingChannel struct {
	mu       sync.Mutex
	chanType string
	err      error
	attempts []domain.Notification
}

func (c *failingChannel) Type() string { return c.chanType }

func (c *failingChannel) Send(_ context.Context, n domain.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempts = append(c.attempts, n)
	return c.err
}

// ── Helpers ─────────────────────────────────────────────────────────────

func newTestService(store *memNotificationStore, rules *memRuleStore) (*Service, *[]string) {
	var broadcasts []string
	svc := NewService(store, rules, func(method string, params any) {
		broadcasts = append(broadcasts, method)
	}, nil)
	return svc, &broadcasts
}

func makeWatermillMsg(t testing.TB, event types.EventRecord) *message.Message {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return message.NewMessage(uuid.NewString(), payload)
}

// ── Tests ───────────────────────────────────────────────────────────────

func TestHandleEvent_RoutesToEvaluator(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{}
	svc, broadcasts := newTestService(store, rules)

	// Register an evaluator that produces one notification for any event
	svc.RegisterEvaluator(&stubEvaluator{
		source: "test",
		result: []domain.Notification{
			{UserID: "prof-1", Source: "test", Trigger: "test_trigger", Severity: domain.SeverityInfo, Title: "Test"},
		},
	})

	event := types.NewEventRecord("run-1", "test.event", "something happened", nil)
	msg := makeWatermillMsg(t, event)
	if err := svc.HandleEvent(msg); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	// Notification should be persisted
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.notifications))
	}
	if store.notifications[0].Source != "test" {
		t.Errorf("expected source 'test', got %q", store.notifications[0].Source)
	}

	// Broadcast should fire
	if len(*broadcasts) != 1 || (*broadcasts)[0] != "notification.raised" {
		t.Errorf("expected broadcast 'notification.raised', got %v", *broadcasts)
	}
}

func TestThrottle_SuppressesDuplicateWithinCooldown(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{
		rules: []domain.NotificationRule{
			{ID: "r1", UserID: "prof-1", Source: "heartbeat", Trigger: "*", MinSeverity: domain.SeverityInfo, Channels: []string{"in_app"}, CooldownMinutes: 30, Enabled: true},
		},
	}
	svc, _ := newTestService(store, rules)

	ctx := context.Background()

	// First notification should persist
	n1 := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, ThrottleKey: "check_api", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n1)
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.notifications))
	}

	// Second identical notification within cooldown should be suppressed
	n2 := domain.Notification{
		ID: "n2", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, ThrottleKey: "check_api", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n2)
	if len(store.notifications) != 1 {
		t.Fatalf("expected 1 notification (suppressed), got %d", len(store.notifications))
	}
}

func TestThrottle_EscalationBypassesCooldown(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{
		rules: []domain.NotificationRule{
			{ID: "r1", UserID: "prof-1", Source: "heartbeat", Trigger: "*", MinSeverity: domain.SeverityInfo, Channels: []string{"in_app"}, CooldownMinutes: 30, Enabled: true},
		},
	}
	svc, _ := newTestService(store, rules)

	ctx := context.Background()

	// First: warning
	n1 := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_warning",
		Severity: domain.SeverityWarning, ThrottleKey: "check_api", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n1)

	// Second: escalation to critical within cooldown — should NOT be suppressed
	n2 := domain.Notification{
		ID: "n2", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_warning",
		Severity: domain.SeverityCritical, ThrottleKey: "check_api", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n2)
	if len(store.notifications) != 2 {
		t.Fatalf("expected 2 notifications (escalation bypass), got %d", len(store.notifications))
	}
}

func TestThrottle_DifferentKeysFireIndependently(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{
		rules: []domain.NotificationRule{
			{ID: "r1", UserID: "prof-1", Source: "heartbeat", Trigger: "*", MinSeverity: domain.SeverityInfo, Channels: []string{"in_app"}, CooldownMinutes: 30, Enabled: true},
		},
	}
	svc, _ := newTestService(store, rules)

	ctx := context.Background()

	n1 := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, ThrottleKey: "check_api", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n1)

	// Different throttle key should fire independently
	n2 := domain.Notification{
		ID: "n2", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, ThrottleKey: "check_watchlist", CreatedAt: time.Now(),
	}
	svc.raise(ctx, n2)
	if len(store.notifications) != 2 {
		t.Fatalf("expected 2 notifications (different keys), got %d", len(store.notifications))
	}
}

func TestRuleMatching_DispatchesToChannels(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{
		rules: []domain.NotificationRule{
			{ID: "r1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical", MinSeverity: domain.SeverityCritical, Channels: []string{"in_app", "webhook"}, CooldownMinutes: 0, Enabled: true},
		},
	}
	svc, _ := newTestService(store, rules)

	inApp := &stubChannel{chanType: "in_app"}
	webhook := &stubChannel{chanType: "webhook"}
	svc.RegisterChannel(inApp)
	svc.RegisterChannel(webhook)

	ctx := context.Background()
	n := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, CreatedAt: time.Now(),
	}
	svc.raise(ctx, n)

	if len(inApp.sent) != 1 {
		t.Errorf("expected 1 in_app delivery, got %d", len(inApp.sent))
	}
	if len(webhook.sent) != 1 {
		t.Errorf("expected 1 webhook delivery, got %d", len(webhook.sent))
	}
}

// TestRuleMatching_PartialDeliveryContinuesOnChannelFailure verifies that when one
// channel returns an error, the service still attempts to dispatch to remaining
// channels for the same rule (partial delivery does not abort the loop).
func TestRuleMatching_PartialDeliveryContinuesOnChannelFailure(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{
		rules: []domain.NotificationRule{
			{ID: "r1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical", MinSeverity: domain.SeverityCritical, Channels: []string{"webhook", "in_app"}, CooldownMinutes: 0, Enabled: true},
		},
	}
	svc, _ := newTestService(store, rules)

	failing := &failingChannel{chanType: "webhook", err: errors.New("boom")}
	succeeding := &stubChannel{chanType: "in_app"}
	svc.RegisterChannel(failing)
	svc.RegisterChannel(succeeding)

	ctx := context.Background()
	n := domain.Notification{
		ID: "n1", UserID: "prof-1", Source: "heartbeat", Trigger: "outcome_critical",
		Severity: domain.SeverityCritical, CreatedAt: time.Now(),
	}
	svc.raise(ctx, n)

	if len(failing.attempts) != 1 {
		t.Errorf("failing channel attempts = %d, want 1", len(failing.attempts))
	}
	if len(succeeding.sent) != 1 {
		t.Errorf("succeeding channel sent = %d, want 1 (partial delivery must continue)", len(succeeding.sent))
	}
}

func TestSeedDefaultRules(t *testing.T) {
	rules := &memRuleStore{}
	svc, _ := newTestService(&memNotificationStore{}, rules)

	ctx := context.Background()
	if err := svc.SeedDefaultRules(ctx, "prof-1"); err != nil {
		t.Fatalf("SeedDefaultRules: %v", err)
	}

	if len(rules.rules) != 5 {
		t.Fatalf("expected 5 default rules, got %d", len(rules.rules))
	}

	// Calling again should be a no-op
	if err := svc.SeedDefaultRules(ctx, "prof-1"); err != nil {
		t.Fatalf("SeedDefaultRules (second): %v", err)
	}
	if len(rules.rules) != 5 {
		t.Fatalf("expected 5 rules after idempotent seed, got %d", len(rules.rules))
	}
}

func TestMarkRead_Broadcasts(t *testing.T) {
	store := &memNotificationStore{
		notifications: []domain.Notification{
			{ID: "n1", UserID: "prof-1", Source: "test", CreatedAt: time.Now()},
		},
	}
	svc, broadcasts := newTestService(store, &memRuleStore{})

	if err := svc.MarkRead(context.Background(), "n1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	if store.notifications[0].ReadAt == nil {
		t.Error("expected notification to be marked as read")
	}
	if len(*broadcasts) != 1 || (*broadcasts)[0] != "notification.read" {
		t.Errorf("expected broadcast 'notification.read', got %v", *broadcasts)
	}
}

func TestUnreadCount(t *testing.T) {
	store := &memNotificationStore{
		notifications: []domain.Notification{
			{ID: "n1", UserID: "prof-1", Source: "test", CreatedAt: time.Now()},
			{ID: "n2", UserID: "prof-1", Source: "test", CreatedAt: time.Now()},
			{ID: "n3", UserID: "prof-1", Source: "test", CreatedAt: time.Now(), ReadAt: timePtr(time.Now())},
		},
	}
	svc, _ := newTestService(store, &memRuleStore{})

	count, err := svc.UnreadCount(context.Background(), "prof-1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread, got %d", count)
	}
}

func TestRegisteredSources(t *testing.T) {
	svc, _ := newTestService(&memNotificationStore{}, &memRuleStore{})
	svc.RegisterEvaluator(&stubEvaluator{source: "heartbeat"})
	svc.RegisterEvaluator(&stubEvaluator{source: "task"})

	sources := svc.RegisteredSources()
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// stubResolverEvaluator implements both TriggerEvaluator and SubjectResolver
// — the shape adopters like HumanInputNotificationEvaluator use to plug
// into the auto-dismiss path.
type stubResolverEvaluator struct {
	source     string
	dismissals []domain.SubjectDismissal
}

func (e *stubResolverEvaluator) Source() string { return e.source }

func (e *stubResolverEvaluator) Evaluate(_ context.Context, _ types.EventRecord) []domain.Notification {
	return nil
}

func (e *stubResolverEvaluator) Resolve(_ context.Context, _ types.EventRecord) []domain.SubjectDismissal {
	return e.dismissals
}

// TestHandleEvent_RoutesResolverToDismissBySubject pins the service-side
// half of auto-dismiss-on-resolve: when an evaluator implements
// SubjectResolver, HandleEvent must collect its dismissals and apply
// them through DismissBySubject. Without this routing, evaluators
// emitting dismissals would be silently ignored — the same class of
// dead-relay bug we hit before with decision.logged.
func TestHandleEvent_RoutesResolverToDismissBySubject(t *testing.T) {
	store := &memNotificationStore{}
	rules := &memRuleStore{}
	svc, broadcasts := newTestService(store, rules)

	subject := domain.Subject{Kind: "human_input", ID: "space-1:member-fred:tool-call-9"}

	// Seed an active notification matching the dismissal scope.
	store.notifications = append(store.notifications, domain.Notification{
		ID:        "existing-1",
		UserID:    "user-a",
		Source:    "human_input",
		Trigger:   "ask_user_pending",
		Severity:  domain.SeverityCritical,
		Subject:   subject,
		Title:     "Question pending",
		CreatedAt: time.Now(),
	})
	// And one that should NOT be dismissed (different subject).
	store.notifications = append(store.notifications, domain.Notification{
		ID:        "untouched-1",
		UserID:    "user-a",
		Source:    "human_input",
		Trigger:   "ask_user_pending",
		Severity:  domain.SeverityCritical,
		Subject:   domain.Subject{Kind: "human_input", ID: "different"},
		Title:     "Other question",
		CreatedAt: time.Now(),
	})

	svc.RegisterEvaluator(&stubResolverEvaluator{
		source: "human_input",
		dismissals: []domain.SubjectDismissal{{
			UserID: "user-a", Source: "human_input", Subject: subject,
		}},
	})

	event := types.NewEventRecord("run-1", "human_input.resolved", "resolved", nil)
	if err := svc.HandleEvent(makeWatermillMsg(t, event)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}

	// The matching row should be dismissed.
	if store.notifications[0].DismissedAt == nil {
		t.Errorf("matching notification not dismissed")
	}
	// The non-matching row stays active.
	if store.notifications[1].DismissedAt != nil {
		t.Errorf("non-matching notification was incorrectly dismissed")
	}
	// And the dismiss-by-subject broadcast should fire (no notification.raised
	// since Evaluate returned nil).
	found := false
	for _, b := range *broadcasts {
		if b == "notification.dismissed_by_subject" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected broadcast 'notification.dismissed_by_subject', got %v", *broadcasts)
	}
}

// TestHandleEvent_ResolverWithNoMatchingRowsSkipsBroadcast verifies the
// service doesn't emit a dismiss broadcast when DismissBySubject affected
// zero rows. Otherwise every "resolved" event would spam the websocket
// with empty dismissals, regardless of whether a row actually existed.
func TestHandleEvent_ResolverWithNoMatchingRowsSkipsBroadcast(t *testing.T) {
	store := &memNotificationStore{} // empty
	rules := &memRuleStore{}
	svc, broadcasts := newTestService(store, rules)

	svc.RegisterEvaluator(&stubResolverEvaluator{
		source: "human_input",
		dismissals: []domain.SubjectDismissal{{
			UserID: "user-a", Source: "human_input",
			Subject: domain.Subject{Kind: "human_input", ID: "nonexistent"},
		}},
	})

	event := types.NewEventRecord("run-1", "human_input.resolved", "resolved", nil)
	if err := svc.HandleEvent(makeWatermillMsg(t, event)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	for _, b := range *broadcasts {
		if b == "notification.dismissed_by_subject" {
			t.Errorf("dismiss broadcast fired with zero affected rows: %v", *broadcasts)
		}
	}
}

// TestHandleEvent_ResolverWithIncompleteSubjectIsDropped pins the
// service guard against partial dismissal triples: a missing UserID,
// Source, or Subject component would otherwise let DismissBySubject
// match too broadly (any "" component is implicitly a wildcard against
// real-world keys that always populate them).
func TestHandleEvent_ResolverWithIncompleteSubjectIsDropped(t *testing.T) {
	store := &memNotificationStore{}
	store.notifications = append(store.notifications, domain.Notification{
		ID: "n-1", UserID: "user-a", Source: "human_input",
		Subject:   domain.Subject{Kind: "human_input", ID: "id-1"},
		CreatedAt: time.Now(),
	})
	rules := &memRuleStore{}
	svc, _ := newTestService(store, rules)

	svc.RegisterEvaluator(&stubResolverEvaluator{
		source: "human_input",
		dismissals: []domain.SubjectDismissal{
			{UserID: "", Source: "human_input", Subject: domain.Subject{Kind: "human_input", ID: "id-1"}},
			{UserID: "user-a", Source: "", Subject: domain.Subject{Kind: "human_input", ID: "id-1"}},
			{UserID: "user-a", Source: "human_input", Subject: domain.Subject{Kind: "", ID: "id-1"}},
			{UserID: "user-a", Source: "human_input", Subject: domain.Subject{Kind: "human_input", ID: ""}},
		},
	})

	event := types.NewEventRecord("run-1", "human_input.resolved", "resolved", nil)
	if err := svc.HandleEvent(makeWatermillMsg(t, event)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if store.notifications[0].DismissedAt != nil {
		t.Errorf("notification dismissed by partial-triple dismissal — guard regressed")
	}
}
