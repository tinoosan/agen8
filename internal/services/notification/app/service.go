// Package app implements the notification application service.
// It orchestrates evaluator registration, rule matching, throttle dedup,
// channel dispatch, and retention pruning.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// BroadcastFunc pushes a notification to connected clients via SSE/WebSocket.
type BroadcastFunc func(method string, params any)

// Service is the core application service for the notification bounded context.
// It routes domain events through registered evaluators, matches against operator rules,
// applies throttle dedup, dispatches to channels, and manages retention.
type Service struct {
	mu         sync.RWMutex
	evaluators map[string]domain.TriggerEvaluator    // registered by source name
	channels   map[string]domain.NotificationChannel // registered by channel type
	rules      domain.NotificationRuleRepository     // operator-configured rules
	store      domain.NotificationRepository         // persists notifications
	broadcast  BroadcastFunc                         // push to connected clients
	retention  domain.RetentionPolicy
	logger     *slog.Logger
}

// NewService creates a new Service.
func NewService(
	store domain.NotificationRepository,
	rules domain.NotificationRuleRepository,
	broadcast BroadcastFunc,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		evaluators: make(map[string]domain.TriggerEvaluator),
		channels:   make(map[string]domain.NotificationChannel),
		rules:      rules,
		store:      store,
		broadcast:  broadcast,
		retention:  domain.DefaultRetentionPolicy(),
		logger:     logger,
	}
}

// RegisterEvaluator adds a domain-specific trigger evaluator.
// Called at startup by each bounded context that wants to raise notifications.
// This is the OCP extension point — no core modification needed.
func (s *Service) RegisterEvaluator(e domain.TriggerEvaluator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluators[e.Source()] = e
	s.logger.Info("registered notification evaluator", "source", e.Source())
}

// RegisterChannel adds a delivery channel.
// This is the OCP extension point for delivery mechanisms.
func (s *Service) RegisterChannel(c domain.NotificationChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[c.Type()] = c
	s.logger.Info("registered notification channel", "type", c.Type())
}

// RegisteredSources returns the source names of all registered evaluators.
// Used by the frontend to auto-populate notification preferences.
func (s *Service) RegisteredSources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sources := make([]string, 0, len(s.evaluators))
	for src := range s.evaluators {
		sources = append(sources, src)
	}
	return sources
}

// RegisteredChannelTypes returns the type names of all registered channels.
func (s *Service) RegisteredChannelTypes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	types := make([]string, 0, len(s.channels))
	for t := range s.channels {
		types = append(types, t)
	}
	return types
}

// HandleEvent is the Watermill event handler. It receives all domain events,
// routes them to every registered evaluator, and raises any resulting notifications.
// Evaluators that don't recognize the event return nil — no wasted work.
func (s *Service) HandleEvent(msg *message.Message) error {
	var event types.EventRecord
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		s.logger.Warn("notification handler: failed to unmarshal event", "error", err)
		msg.Ack()
		return nil
	}

	ctx := context.Background()
	s.mu.RLock()
	evaluators := make([]domain.TriggerEvaluator, 0, len(s.evaluators))
	for _, e := range s.evaluators {
		evaluators = append(evaluators, e)
	}
	s.mu.RUnlock()

	for _, evaluator := range evaluators {
		notifications := evaluator.Evaluate(ctx, event)
		for i := range notifications {
			if notifications[i].ID == "" {
				notifications[i].ID = uuid.NewString()
			}
			if notifications[i].CreatedAt.IsZero() {
				notifications[i].CreatedAt = time.Now()
			}
			s.raise(ctx, notifications[i])
		}
	}

	// Second pass: any evaluator that opted into SubjectResolver gets a
	// chance to declare auto-dismissals for this event (e.g. human_input
	// resolves the pending question). We route through the same
	// DismissBySubject path UI/RPC use so persistence and broadcast stay
	// in sync.
	for _, evaluator := range evaluators {
		resolver, ok := evaluator.(domain.SubjectResolver)
		if !ok {
			continue
		}
		for _, d := range resolver.Resolve(ctx, event) {
			s.applyDismissal(ctx, d)
		}
	}

	msg.Ack()
	return nil
}

// applyDismissal clears all active notifications matching the dismissal
// triple and broadcasts a dismiss event when at least one row was hidden.
// Errors are logged, never returned — auto-dismiss is a convenience layer
// over manual dismiss; the user can still clear the row themselves.
func (s *Service) applyDismissal(ctx context.Context, d domain.SubjectDismissal) {
	if d.UserID == "" || d.Source == "" || d.Subject.Kind == "" || d.Subject.ID == "" {
		// Without a fully scoped triple we'd risk dismissing unrelated
		// rows. Skip rather than guess.
		return
	}
	affected, err := s.store.DismissBySubject(ctx, d.UserID, d.Source, d.Subject)
	if err != nil {
		s.logger.Warn("auto-dismiss failed",
			"userId", d.UserID, "source", d.Source,
			"subjectKind", d.Subject.Kind, "subjectId", d.Subject.ID,
			"error", err)
		return
	}
	if affected > 0 && s.broadcast != nil {
		s.broadcast("notification.dismissed_by_subject", map[string]any{
			"userId":      d.UserID,
			"source":      d.Source,
			"subjectKind": d.Subject.Kind,
			"subjectId":   d.Subject.ID,
			"affected":    affected,
		})
	}
}

// raise persists the notification, applies throttle dedup, matches against rules,
// and dispatches to the appropriate channels.
func (s *Service) raise(ctx context.Context, n domain.Notification) {
	// Check throttle: suppress duplicate notifications within cooldown window
	if n.ThrottleKey != "" {
		last, err := s.store.LastByThrottleKey(ctx, n.UserID, n.Source, n.Trigger, n.ThrottleKey)
		if err != nil {
			s.logger.Warn("throttle check failed", "error", err)
			// Continue — fail open rather than suppress valid notifications
		} else if last != nil {
			cooldown := s.effectiveCooldown(ctx, n)
			withinCooldown := time.Since(last.CreatedAt) < cooldown
			notEscalated := domain.SeverityRank(n.Severity) <= domain.SeverityRank(last.Severity)

			if withinCooldown && notEscalated {
				s.logger.Debug("notification suppressed by throttle",
					"source", n.Source, "trigger", n.Trigger,
					"throttleKey", n.ThrottleKey, "cooldown", cooldown)
				return
			}
		}
	}

	if err := s.store.Save(ctx, n); err != nil {
		s.logger.Error("failed to save notification", "error", err, "id", n.ID)
		return
	}

	s.mu.RLock()
	channels := s.channels
	s.mu.RUnlock()

	rules, err := s.rules.FindMatching(ctx, n.UserID, n.Source, n.Trigger, n.Severity)
	if err != nil {
		s.logger.Warn("rule matching failed", "error", err)
	}
	for _, rule := range rules {
		dispatchN := n
		if rule.WebhookURL != "" {
			if dispatchN.Metadata == nil {
				dispatchN.Metadata = map[string]string{}
			}
			dispatchN.Metadata["webhookURL"] = rule.WebhookURL
		}
		for _, chType := range rule.Channels {
			if ch, ok := channels[chType]; ok {
				if sendErr := ch.Send(ctx, dispatchN); sendErr != nil {
					s.logger.Error("channel send failed",
						"channel", chType, "error", sendErr, "notificationId", n.ID)
				}
			}
		}
	}

	if s.broadcast != nil {
		s.broadcast("notification.raised", n)
	}
}

// effectiveCooldown returns the cooldown duration for a notification.
func (s *Service) effectiveCooldown(ctx context.Context, n domain.Notification) time.Duration {
	rules, err := s.rules.FindMatching(ctx, n.UserID, n.Source, n.Trigger, n.Severity)
	if err != nil || len(rules) == 0 {
		return 30 * time.Minute
	}
	if rules[0].CooldownMinutes > 0 {
		return time.Duration(rules[0].CooldownMinutes) * time.Minute
	}
	return 30 * time.Minute
}

// ── Query methods (delegated to store) ──────────────────────────────────

// List returns notifications for a user, applying the given filter.
func (s *Service) List(ctx context.Context, userID string, filter domain.NotificationFilter) ([]domain.Notification, error) {
	return s.store.FindByUser(ctx, userID, filter)
}

// UnreadCount returns the number of unread notifications for a user.
func (s *Service) UnreadCount(ctx context.Context, userID string) (int, error) {
	return s.store.UnreadCount(ctx, userID)
}

// MarkRead marks a single notification as read.
func (s *Service) MarkRead(ctx context.Context, id string) error {
	if err := s.store.MarkRead(ctx, id); err != nil {
		return err
	}
	if s.broadcast != nil {
		s.broadcast("notification.read", map[string]string{"id": id})
	}
	return nil
}

// MarkAllRead marks all notifications for a user as read.
func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	if err := s.store.MarkAllRead(ctx, userID); err != nil {
		return err
	}
	if s.broadcast != nil {
		s.broadcast("notification.read", map[string]string{"userId": userID, "all": "true"})
	}
	return nil
}

// Dismiss dismisses a notification (hides it from the active list).
func (s *Service) Dismiss(ctx context.Context, id string) error {
	return s.store.Dismiss(ctx, id)
}

// DismissBySubject auto-dismisses every active notification matching
// (userID, source, subject). Returns rows affected. Used by domain
// services on resolve to clear stale "thing X needs attention" rows
// without requiring evaluators to coordinate stringly-typed throttle
// keys for the same purpose.
func (s *Service) DismissBySubject(ctx context.Context, userID, source string, subject domain.Subject) (int, error) {
	return s.store.DismissBySubject(ctx, userID, source, subject)
}

// ── Rule management ─────────────────────────────────────────────────────

// ListRules returns all notification rules for a user.
func (s *Service) ListRules(ctx context.Context, userID string) ([]domain.NotificationRule, error) {
	return s.rules.FindByUser(ctx, userID)
}

// SaveRule creates or updates a notification rule.
func (s *Service) SaveRule(ctx context.Context, rule domain.NotificationRule) error {
	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	return s.rules.Save(ctx, rule)
}

// DeleteRule removes a notification rule.
func (s *Service) DeleteRule(ctx context.Context, id string) error {
	return s.rules.Delete(ctx, id)
}

// SeedDefaultRules creates the default notification rules for a user
// if no rules exist yet.
func (s *Service) SeedDefaultRules(ctx context.Context, userID string) error {
	existing, err := s.rules.FindByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("check existing rules: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	for _, rule := range domain.DefaultRules(userID) {
		rule.ID = uuid.NewString()
		if err := s.rules.Save(ctx, rule); err != nil {
			return fmt.Errorf("seed rule: %w", err)
		}
	}
	return nil
}

// ── Retention ───────────────────────────────────────────────────────────

// StartRetentionLoop runs the retention prune pass on a fixed interval.
// Call this in a goroutine at startup. It blocks until ctx is cancelled.
func (s *Service) StartRetentionLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruned, err := s.store.Prune(ctx, s.retention)
			if err != nil {
				s.logger.Warn("retention prune failed", "error", err)
			} else if pruned > 0 {
				s.logger.Info("retention prune completed", "pruned", pruned)
			}
		}
	}
}
