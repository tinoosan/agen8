package domain

import (
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type EntryStatus string

const (
	EntryStatusActive    EntryStatus = "active"
	EntryStatusPaused    EntryStatus = "paused"
	EntryStatusTriggered EntryStatus = "triggered"
	EntryStatusExpired   EntryStatus = "expired"
	EntryStatusCancelled EntryStatus = "cancelled"
)

type Context struct {
	MissionID     string `json:"missionId,omitempty"`
	KeyResultID   string `json:"keyResultId,omitempty"`
	PlanPhaseID   string `json:"planPhaseId,omitempty"`
	PlanTodoID    string `json:"planTodoId,omitempty"`
	RelatedTaskID string `json:"relatedTaskId,omitempty"`
}

type Entry struct {
	ID          EntryID             `json:"id"`
	SpaceID     spacedomain.SpaceID `json:"spaceId"`
	CreatedBy   ActorRef            `json:"createdBy"`
	Status      EntryStatus         `json:"status"`
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Timing      TimingExpression    `json:"timing"`
	Target      Target              `json:"target"`
	Context     Context             `json:"context,omitempty"`
	NextRunAt   *time.Time          `json:"nextRunAt,omitempty"`
	ExpiresAt   *time.Time          `json:"expiresAt,omitempty"`
	DedupeKey   string              `json:"dedupeKey,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type NewEntryInput struct {
	ID          EntryID
	SpaceID     spacedomain.SpaceID
	CreatedBy   ActorRef
	Title       string
	Description string
	Timing      TimingExpression
	Target      Target
	Context     Context
	ExpiresAt   *time.Time
	DedupeKey   string
}

func NewEntry(input NewEntryInput, now time.Time) (Entry, error) {
	now = now.UTC()
	id := normalizeEntryID(input.ID)
	if id == "" {
		generated, err := NewEntryID()
		if err != nil {
			return Entry{}, err
		}
		id = generated
	}
	entry := Entry{
		ID:          id,
		SpaceID:     spacedomain.SpaceID(strings.TrimSpace(string(input.SpaceID))),
		CreatedBy:   normalizeActorRef(input.CreatedBy),
		Status:      EntryStatusActive,
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Timing:      input.Timing.normalized(),
		Target:      input.Target.normalized(),
		Context:     input.Context.normalized(),
		ExpiresAt:   normalizeTimePtr(input.ExpiresAt),
		DedupeKey:   strings.TrimSpace(input.DedupeKey),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}
	next, err := entry.Timing.FirstRunAfter(now)
	if err != nil {
		return Entry{}, err
	}
	if entry.ExpiresAt != nil && !next.Before(*entry.ExpiresAt) {
		entry.Status = EntryStatusExpired
	} else {
		next = next.UTC()
		entry.NextRunAt = &next
	}
	return entry, nil
}

func (e Entry) Validate() error {
	if strings.TrimSpace(string(e.ID)) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(string(e.SpaceID)) == "" {
		return fmt.Errorf("spaceId is required")
	}
	if strings.TrimSpace(string(e.CreatedBy)) == "" {
		return fmt.Errorf("createdBy is required")
	}
	if strings.TrimSpace(e.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if err := validateEntryStatus(e.Status); err != nil {
		return err
	}
	if err := e.Timing.Validate(); err != nil {
		return err
	}
	if err := e.Target.Validate(); err != nil {
		return err
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("createdAt is required")
	}
	if e.UpdatedAt.IsZero() {
		return fmt.Errorf("updatedAt is required")
	}
	return nil
}

func (e Entry) IsDue(now time.Time) bool {
	return e.Status == EntryStatusActive && e.NextRunAt != nil && !e.NextRunAt.After(now.UTC())
}

func (e Entry) Cancel(now time.Time) (Entry, error) {
	if e.Status == EntryStatusCancelled {
		return Entry{}, fmt.Errorf("schedule entry is already cancelled")
	}
	if e.Status == EntryStatusTriggered || e.Status == EntryStatusExpired {
		return Entry{}, fmt.Errorf("cannot cancel schedule entry with status %q", e.Status)
	}
	next := e
	next.Status = EntryStatusCancelled
	next.NextRunAt = nil
	next.UpdatedAt = now.UTC()
	return next, nil
}

func (e Entry) AdvanceAfterRun(dueAt, now time.Time) (Entry, error) {
	if e.Status != EntryStatusActive {
		return Entry{}, fmt.Errorf("cannot advance schedule entry with status %q", e.Status)
	}
	next := e
	nextRun, ok, err := e.Timing.NextRunAfter(dueAt.UTC())
	if err != nil {
		return Entry{}, err
	}
	if !ok {
		next.Status = EntryStatusTriggered
		next.NextRunAt = nil
		next.UpdatedAt = now.UTC()
		return next, nil
	}
	if e.ExpiresAt != nil && !nextRun.Before(*e.ExpiresAt) {
		next.Status = EntryStatusExpired
		next.NextRunAt = nil
		next.UpdatedAt = now.UTC()
		return next, nil
	}
	nextRun = nextRun.UTC()
	next.NextRunAt = &nextRun
	next.UpdatedAt = now.UTC()
	return next, nil
}

func validateEntryStatus(status EntryStatus) error {
	switch status {
	case EntryStatusActive, EntryStatusPaused, EntryStatusTriggered, EntryStatusExpired, EntryStatusCancelled:
		return nil
	default:
		return fmt.Errorf("invalid schedule entry status %q", status)
	}
}

func (c Context) normalized() Context {
	return Context{
		MissionID:     strings.TrimSpace(c.MissionID),
		KeyResultID:   strings.TrimSpace(c.KeyResultID),
		PlanPhaseID:   strings.TrimSpace(c.PlanPhaseID),
		PlanTodoID:    strings.TrimSpace(c.PlanTodoID),
		RelatedTaskID: strings.TrimSpace(c.RelatedTaskID),
	}
}

func normalizeTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	next := t.UTC()
	return &next
}
