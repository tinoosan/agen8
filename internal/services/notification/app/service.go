// Package app orchestrates notification delivery: it derives what the user
// should see from the current task snapshot, reconciles that against what's
// already persisted, and exposes read/dismiss operations for the inbox.
//
// Reconciliation runs lazily on SyncAndList (called by notification.list), so
// there is no background ticker. The frontend re-fetches notification.list
// whenever a task.* SSE event invalidates its query, which means a task
// changing state flows straight through to a fresh inbox — delivery rides the
// existing live path.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/services/notification/domain"
)

// TaskSource supplies the current task snapshot for a project. The app layer
// adapts the task service to this narrow port so the notification package never
// imports the task domain directly.
type TaskSource interface {
	Tasks(ctx context.Context, projectID string) ([]domain.TaskSnapshot, error)
}

// Service reconciles derived notifications against the repository.
type Service struct {
	repo   domain.Repository
	tasks  TaskSource
	clock  domain.Clock
	cfg    domain.DeriveConfig
	source string
	logger *slog.Logger
}

// NewService builds the notification service. A nil clock falls back to the
// system clock; a nil logger to a discard logger.
func NewService(repo domain.Repository, tasks TaskSource, clock domain.Clock, cfg domain.DeriveConfig, logger *slog.Logger) (*Service, error) {
	if repo == nil {
		return nil, fmt.Errorf("notification service: repository is required")
	}
	if tasks == nil {
		return nil, fmt.Errorf("notification service: task source is required")
	}
	if clock == nil {
		clock = domain.SystemClock{}
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		repo:   repo,
		tasks:  tasks,
		clock:  clock,
		cfg:    cfg,
		source: domain.Source,
		logger: logger,
	}, nil
}

// SyncResult is the inbox payload: the active notifications plus the unread
// count (unread = active and never read).
type SyncResult struct {
	Notifications []domain.Notification
	UnreadCount   int
}

// SyncAndList derives the current notifications for a user/project, reconciles
// them into the store, and returns the resulting active inbox.
func (s *Service) SyncAndList(ctx context.Context, userID, projectID string) (SyncResult, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" {
		return SyncResult{}, fmt.Errorf("notification sync: user id is required")
	}
	if projectID == "" {
		return SyncResult{}, fmt.Errorf("notification sync: project id is required")
	}

	now := s.clock.Now()
	tasks, err := s.tasks.Tasks(ctx, projectID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("load tasks: %w", err)
	}

	specs := domain.Derive(projectID, tasks, now, s.cfg)

	active, err := s.repo.ListActive(ctx, userID, projectID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list active: %w", err)
	}
	activeByKey := make(map[string]domain.Notification, len(active))
	for _, n := range active {
		if n.ThrottleKey != "" {
			activeByKey[n.ThrottleKey] = n
		}
	}

	desiredKeys := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		desiredKeys[spec.ThrottleKey] = struct{}{}

		// Already showing this exact condition/event — leave it be.
		if _, shown := activeByKey[spec.ThrottleKey]; shown {
			continue
		}
		// One-time events are told once: if we ever created this (even if the
		// user has since dismissed it), don't resurrect it.
		if spec.Kind == domain.KindEvent {
			exists, err := s.repo.ExistsByThrottleKey(ctx, userID, projectID, spec.ThrottleKey)
			if err != nil {
				return SyncResult{}, fmt.Errorf("throttle check: %w", err)
			}
			if exists {
				continue
			}
		}
		if err := s.repo.Insert(ctx, s.buildNotification(spec, userID, projectID, now)); err != nil {
			return SyncResult{}, fmt.Errorf("insert notification: %w", err)
		}
	}

	// Auto-dismiss standing nudges whose condition no longer appears in the
	// derived set — the queue drained, the task got picked up, etc.
	for _, n := range active {
		if n.ThrottleKey == "" || !domain.IsStandingTrigger(n.Trigger) {
			continue
		}
		if _, stillDesired := desiredKeys[n.ThrottleKey]; stillDesired {
			continue
		}
		if err := s.repo.Dismiss(ctx, userID, n.ID); err != nil {
			return SyncResult{}, fmt.Errorf("auto-dismiss notification: %w", err)
		}
	}

	final, err := s.repo.ListActive(ctx, userID, projectID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("relist active: %w", err)
	}
	unread := 0
	for _, n := range final {
		if n.ReadAt == nil {
			unread++
		}
	}
	return SyncResult{Notifications: final, UnreadCount: unread}, nil
}

// MarkRead clears the unread state on one notification.
func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("notification mark read: id is required")
	}
	return s.repo.MarkRead(ctx, userID, id)
}

// MarkAllRead clears unread state across a project's inbox; returns the count.
func (s *Service) MarkAllRead(ctx context.Context, userID, projectID string) (int, error) {
	return s.repo.MarkAllRead(ctx, userID, projectID)
}

// Dismiss removes a notification from the active inbox.
func (s *Service) Dismiss(ctx context.Context, userID, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("notification dismiss: id is required")
	}
	return s.repo.Dismiss(ctx, userID, id)
}

func (s *Service) buildNotification(spec domain.Spec, userID, projectID string, now time.Time) domain.Notification {
	return domain.Notification{
		ID:          "ntf-" + uuid.NewString(),
		UserID:      userID,
		ProjectID:   projectID,
		Source:      s.source,
		Trigger:     spec.Trigger,
		Severity:    spec.Severity,
		SubjectKind: spec.SubjectKind,
		SubjectID:   spec.SubjectID,
		Title:       spec.Title,
		Body:        spec.Body,
		LinkSurface: spec.LinkSurface,
		LinkURL:     spec.LinkURL,
		ThrottleKey: spec.ThrottleKey,
		Metadata:    spec.Metadata,
		CreatedAt:   now.UTC(),
	}
}
