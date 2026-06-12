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
	// KindAsking — the agent posed an interactive question and is blocked on
	// the answer. Kind taxonomy only, no question content, so harnesses without
	// an observable question tool stay at parity.
	KindAsking Kind = "asking"
	// KindCleared — the session resumed; not a state, only an event.
	KindCleared Kind = "cleared"
)

// DefaultTTL bounds how long a stale entry survives without a clear signal —
// the crash ghost: a killed session never sends cleared. Two hours keeps a
// genuine overnight wait visible long enough to act on while capping how long
// a ghost can lie.
const DefaultTTL = 2 * time.Hour

// Event is the normalized payload a harness hook posts.
type Event struct {
	SessionRef string `json:"sessionRef"`
	Harness    string `json:"harness,omitempty"`
	Kind       Kind   `json:"kind"`
	Message    string `json:"message,omitempty"`
	// Cwd is the session's working directory from the harness payload, used to
	// attribute sessions that registered no member (cwd -> project root).
	Cwd string `json:"cwd,omitempty"`
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

// ProjectLookup resolves a working directory to the project whose root
// contains it — the fallback attribution for sessions with no registered
// member (e.g. Codex's user-level hooks firing from any directory).
type ProjectLookup interface {
	ProjectIDForDir(ctx context.Context, dir string) (string, bool)
}

// Service is the in-memory attention tracker, keyed by session ref.
type Service struct {
	mu      sync.RWMutex
	entries map[string]Entry

	members  MemberLookup
	projects ProjectLookup
	events   EventPublisher
	now      func() time.Time
	ttl      time.Duration
	logger   *slog.Logger
}

// NewService builds the tracker. members and events are required; projects is
// optional (without it, member-unmatched events are dropped outright); now and
// ttl default to time.Now and DefaultTTL.
func NewService(members MemberLookup, projects ProjectLookup, events EventPublisher, now func() time.Time, ttl time.Duration, logger *slog.Logger) (*Service, error) {
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
		entries:  map[string]Entry{},
		members:  members,
		projects: projects,
		events:   events,
		now:      now,
		ttl:      ttl,
		logger:   logger,
	}, nil
}

// Report ingests one hook event for the authenticated user. It never blocks on
// downstream consumers: attribution is best-effort and a bus publish failure is
// logged, not returned — the hook caller must always get a fast, successful ack.
//
// Attribution is two-step: session ref -> registered member, else cwd ->
// project root. An event matching neither is dropped — Codex's user-level
// hooks fire from every directory, and a session agen8 knows nothing about
// must not pollute project dashboards.
func (s *Service) Report(ctx context.Context, userID string, ev Event) (Entry, error) {
	sessionRef := strings.TrimSpace(ev.SessionRef)
	if sessionRef == "" {
		return Entry{}, errors.New("attention report: sessionRef is required")
	}
	switch ev.Kind {
	case KindWaiting, KindNeedsApproval, KindAsking, KindCleared:
	default:
		return Entry{}, fmt.Errorf("attention report: invalid kind %q (must be waiting, needs_approval, asking, or cleared)", ev.Kind)
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
	if entry.ProjectID == "" && s.projects != nil {
		if projectID, ok := s.projects.ProjectIDForDir(ctx, strings.TrimSpace(ev.Cwd)); ok {
			entry.ProjectID = projectID
		}
	}
	if entry.ProjectID == "" {
		s.logger.Debug("attention: drop event from unknown session", "sessionRef", sessionRef, "cwd", ev.Cwd)
		return Entry{}, nil
	}

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

// List returns live entries for a project, newest wait first. Every stored
// entry carries a project (unmappable events are dropped at Report), so the
// scope is exact. Expired entries are swept on the way through.
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
		if entry.ProjectID == projectID {
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
