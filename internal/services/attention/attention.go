// Package attention tracks which harness sessions are waiting on the human.
//
// State here is deliberately ephemeral presence, not work: it lives in memory,
// expires on a TTL, clears on any newer signal from the session, and never
// touches the graph. A question that must survive belongs to the (parked)
// question primitive; this is the radar that says "Sora (Claude Code) needs
// you", nothing more. Detection comes from harness hooks (Claude Code
// Notification/Stop, Codex Stop/PermissionRequest) POSTing normalized events
// keyed by the native session ref captured at project.register.
package attention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

// Kind classifies why a session needs the human.
type Kind string

const (
	// KindWaiting — the agent finished its turn / is idle at the prompt.
	KindWaiting Kind = "waiting"
	// KindNeedsApproval — the agent is blocked on a permission/approval prompt.
	KindNeedsApproval Kind = "needs_approval"
	// KindCleared — the session resumed; not a state, only an event.
	KindCleared Kind = "cleared"
)

// DefaultTTL bounds how long a stale entry survives without a clear signal
// (hook script lost, laptop slept mid-wait). Generous on purpose: a false
// "waiting" that self-expires beats a real wait that vanishes too early.
const DefaultTTL = 6 * time.Hour

// Event is the normalized payload a harness hook posts.
type Event struct {
	SessionRef string `json:"sessionRef"`
	Harness    string `json:"harness,omitempty"`
	Kind       Kind   `json:"kind"`
	Message    string `json:"message,omitempty"`
}

// Entry is one session currently needing the human.
type Entry struct {
	SessionRef string    `json:"sessionRef"`
	UserID     string    `json:"-"`
	ProjectID  string    `json:"projectId,omitempty"`
	MemberID   string    `json:"memberId,omitempty"`
	MemberName string    `json:"memberName,omitempty"`
	Harness    string    `json:"harness,omitempty"`
	Kind       Kind      `json:"kind"`
	Message    string    `json:"message,omitempty"`
	Since      time.Time `json:"since"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// MemberLookup resolves a native session ref to a registered member. Satisfied
// by the project app service's ListMembers.
type MemberLookup interface {
	ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error)
}

// EventPublisher publishes onto the domain event bus.
type EventPublisher interface {
	Publish(topic string, event any) error
}

// Service is the in-memory attention tracker, keyed by session ref.
type Service struct {
	mu      sync.RWMutex
	entries map[string]Entry

	members MemberLookup
	events  EventPublisher
	now     func() time.Time
	ttl     time.Duration
	logger  *slog.Logger
}

// NewService builds the tracker. members and events are required; now and ttl
// default to time.Now and DefaultTTL.
func NewService(members MemberLookup, events EventPublisher, now func() time.Time, ttl time.Duration, logger *slog.Logger) (*Service, error) {
	if members == nil {
		return nil, errors.New("attention service: member lookup is required")
	}
	if events == nil {
		return nil, errors.New("attention service: event publisher is required")
	}
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if logger == nil {
		logger = slog.Default().With("service", "attention")
	}
	return &Service{
		entries: map[string]Entry{},
		members: members,
		events:  events,
		now:     now,
		ttl:     ttl,
		logger:  logger,
	}, nil
}

// Report ingests one hook event for the authenticated user. It never blocks on
// downstream consumers: member resolution is best-effort (an unmatched session
// is kept as unattributed) and a bus publish failure is logged, not returned —
// the hook caller must always get a fast, successful ack.
func (s *Service) Report(ctx context.Context, userID string, ev Event) (Entry, error) {
	sessionRef := strings.TrimSpace(ev.SessionRef)
	if sessionRef == "" {
		return Entry{}, errors.New("attention report: sessionRef is required")
	}
	switch ev.Kind {
	case KindWaiting, KindNeedsApproval, KindCleared:
	default:
		return Entry{}, fmt.Errorf("attention report: invalid kind %q (must be waiting, needs_approval, or cleared)", ev.Kind)
	}
	now := s.now().UTC()

	if ev.Kind == KindCleared {
		s.mu.Lock()
		existing, existed := s.entries[sessionRef]
		delete(s.entries, sessionRef)
		s.mu.Unlock()
		if existed {
			s.publish(eventbus.AttentionEventCleared, existing, now)
		}
		return existing, nil
	}

	entry := Entry{
		SessionRef: sessionRef,
		UserID:     strings.TrimSpace(userID),
		Harness:    strings.TrimSpace(ev.Harness),
		Kind:       ev.Kind,
		Message:    strings.TrimSpace(ev.Message),
		Since:      now,
		UpdatedAt:  now,
	}
	s.attribute(ctx, &entry)

	s.mu.Lock()
	if existing, ok := s.entries[sessionRef]; ok && existing.Kind == entry.Kind {
		// Same wait refreshed — keep the original start so "waiting for 2h"
		// doesn't reset every time a hook re-fires.
		entry.Since = existing.Since
	}
	s.entries[sessionRef] = entry
	s.mu.Unlock()

	s.publish(eventbus.AttentionEventRaised, entry, now)
	return entry, nil
}

// List returns live entries for a project, newest wait first. Unattributed
// entries (no member matched) are included for every project so they are never
// invisible. Expired entries are swept on the way through.
func (s *Service) List(ctx context.Context, projectID string) []Entry {
	_ = ctx
	projectID = strings.TrimSpace(projectID)
	now := s.now().UTC()

	s.mu.Lock()
	out := make([]Entry, 0, len(s.entries))
	for ref, entry := range s.entries {
		if now.Sub(entry.UpdatedAt) > s.ttl {
			delete(s.entries, ref)
			continue
		}
		if entry.ProjectID == projectID || entry.ProjectID == "" {
			out = append(out, entry)
		}
	}
	s.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Since.After(out[j].Since) })
	return out
}

// attribute resolves the session ref to a registered member, best-effort.
func (s *Service) attribute(ctx context.Context, entry *Entry) {
	filter := member.Filter{
		UserID:           entry.UserID,
		NativeSessionRef: entry.SessionRef,
		LifecycleState:   member.LifecycleActive,
	}
	members, err := s.members.ListMembers(ctx, filter)
	if err != nil {
		s.logger.Warn("attention: resolve member by session ref", "sessionRef", entry.SessionRef, "error", err)
		return
	}
	if len(members) == 0 {
		return
	}
	// Same-session duplicates (harness-label drift) collapse to the
	// earliest-registered row, matching the project service's resolution rule.
	picked := members[0]
	for _, m := range members[1:] {
		if m.RegisteredAt.Before(picked.RegisteredAt) {
			picked = m
		}
	}
	entry.ProjectID = picked.ProjectID
	entry.MemberID = string(picked.ID)
	entry.MemberName = picked.DisplayName
	if kind := strings.TrimSpace(picked.HarnessKind); kind != "" {
		entry.Harness = kind
	}
}

func (s *Service) publish(eventType string, entry Entry, now time.Time) {
	// No project means no SSE room to route to; the entry still shows up in
	// List as unattributed.
	if entry.ProjectID == "" {
		return
	}
	if err := s.events.Publish(eventbus.TopicAttentionLifecycle, eventbus.AttentionEvent{
		ProjectID:  entry.ProjectID,
		MemberID:   entry.MemberID,
		MemberName: entry.MemberName,
		Harness:    entry.Harness,
		Kind:       string(entry.Kind),
		EventType:  eventType,
		Timestamp:  now,
	}); err != nil {
		s.logger.Warn("attention: publish lifecycle event", "eventType", eventType, "sessionRef", entry.SessionRef, "error", err)
	}
}
