package domain

import (
	"fmt"
	"strings"
	"time"
)

type SpaceID string

func (id SpaceID) String() string { return string(id) }

const (
	SpaceStatusOpen   = "open"
	SpaceStatusClosed = "closed"
)

type SpaceCustomization struct {
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type SpaceRecord struct {
	ID            SpaceID             `json:"id"`
	UserID        string              `json:"userId,omitempty"`
	ProjectID     string              `json:"projectId,omitempty"`
	Title         string              `json:"title,omitempty"`
	Status        string              `json:"status,omitempty"`
	PlanMode      string              `json:"planMode,omitempty"`
	CreatedAt     time.Time           `json:"createdAt"`
	UpdatedAt     time.Time           `json:"updatedAt,omitempty"`
	Customization *SpaceCustomization `json:"customization,omitempty"`
}

var validSpaceTransitions = map[string]map[string]bool{
	SpaceStatusOpen:   {SpaceStatusClosed: true},
	SpaceStatusClosed: {SpaceStatusOpen: true},
}

type Space struct {
	inner SpaceRecord
}

func WrapSpace(s SpaceRecord) Space {
	if strings.TrimSpace(s.Status) == "" {
		s.Status = SpaceStatusOpen
	}
	return Space{inner: s}
}

func (a Space) Inner() SpaceRecord { return a.inner }
func (a Space) ID() string         { return string(a.inner.ID) }
func (a Space) Status() string     { return a.inner.Status }

func (a Space) Close(now time.Time) (Space, error) {
	return a.transition(SpaceStatusClosed, now)
}

func (a Space) Reopen(now time.Time) (Space, error) {
	return a.transition(SpaceStatusOpen, now)
}

func (a Space) transition(target string, now time.Time) (Space, error) {
	current := a.Status()
	if current == target {
		return a, nil
	}
	allowed, ok := validSpaceTransitions[current]
	if !ok {
		return Space{}, fmt.Errorf("space: no transitions defined for status %q", current)
	}
	if !allowed[target] {
		return Space{}, fmt.Errorf("space: cannot transition from %q to %q", current, target)
	}
	next := a.inner
	next.Status = target
	next.UpdatedAt = now.UTC()
	return Space{inner: next}, nil
}
