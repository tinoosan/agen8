package project

import (
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusOpen     Status = "open"
	StatusPaused   Status = "paused"
	StatusClosed   Status = "closed"
	StatusArchived Status = "archived"
	StatusDeleting Status = "deleting"
)

func validStatus(s Status) bool {
	switch s {
	case StatusDraft, StatusOpen, StatusPaused, StatusClosed, StatusArchived, StatusDeleting:
		return true
	default:
		return false
	}
}

// validTransitions enumerates the live coordination lifecycle. Only the
// transitions the runtime actually drives are wired: pause/resume, close/reopen,
// archive, and the delete guard (move to deleting).
var validTransitions = map[Status]map[Status]bool{
	StatusDraft:    {StatusOpen: true, StatusArchived: true, StatusDeleting: true},
	StatusOpen:     {StatusPaused: true, StatusClosed: true, StatusArchived: true, StatusDeleting: true},
	StatusPaused:   {StatusOpen: true, StatusClosed: true, StatusArchived: true, StatusDeleting: true},
	StatusClosed:   {StatusOpen: true, StatusArchived: true, StatusDeleting: true},
	StatusArchived: {StatusDeleting: true},
	StatusDeleting: {},
}

type Customization struct {
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type Project struct {
	id            types.ProjectID
	locationID    types.LocationID
	root          string
	userID        string
	title         string
	status        Status
	planMode      string
	customization *Customization
	createdAt     time.Time
	updatedAt     time.Time
}

type NewInput struct {
	ID            types.ProjectID
	LocationID    types.LocationID
	Root          string
	UserID        string
	Title         string
	Status        Status
	PlanMode      string
	Customization *Customization
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func New(input NewInput) (Project, error) {
	id := types.ProjectID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return Project{}, fmt.Errorf("project id is required")
	}
	locationID := types.LocationID(strings.TrimSpace(string(input.LocationID)))
	if locationID == "" {
		locationID = "local"
	}
	root := strings.TrimSpace(input.Root)
	if root == "" {
		return Project{}, fmt.Errorf("project root is required")
	}
	status := input.Status
	if status == "" {
		status = StatusOpen
	}
	if !validStatus(status) {
		return Project{}, fmt.Errorf("unsupported project status %q", status)
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		return Project{}, fmt.Errorf("project created at is required")
	}
	updatedAt := input.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	return Project{
		id:            id,
		locationID:    locationID,
		root:          root,
		userID:        strings.TrimSpace(input.UserID),
		title:         strings.TrimSpace(input.Title),
		status:        status,
		planMode:      strings.TrimSpace(input.PlanMode),
		customization: input.Customization,
		createdAt:     createdAt.UTC(),
		updatedAt:     updatedAt.UTC(),
	}, nil
}

func Wrap(record Record) (Project, error) {
	return New(NewInput{
		ID:            record.ID,
		LocationID:    record.LocationID,
		Root:          record.Root,
		UserID:        record.UserID,
		Title:         record.Title,
		Status:        record.Status,
		PlanMode:      record.PlanMode,
		Customization: record.Customization,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	})
}

func (p Project) ID() types.ProjectID           { return p.id }
func (p Project) LocationID() types.LocationID  { return p.locationID }
func (p Project) Root() string                  { return p.root }
func (p Project) UserID() string                { return p.userID }
func (p Project) Title() string                 { return p.title }
func (p Project) Status() Status                { return p.status }
func (p Project) PlanMode() string              { return p.planMode }
func (p Project) Customization() *Customization { return p.customization }
func (p Project) CreatedAt() time.Time          { return p.createdAt }
func (p Project) UpdatedAt() time.Time          { return p.updatedAt }

// Close moves an active project to closed.
func (p Project) Close(now time.Time) (Project, error) {
	return p.transition(StatusClosed, now)
}

// Reopen moves a closed project back to open.
func (p Project) Reopen(now time.Time) (Project, error) {
	return p.transition(StatusOpen, now)
}

// Pause suspends an open project.
func (p Project) Pause(now time.Time) (Project, error) {
	return p.transition(StatusPaused, now)
}

// Resume returns a paused project to open.
func (p Project) Resume(now time.Time) (Project, error) {
	return p.transition(StatusOpen, now)
}

// Archive moves a project to the archived terminal state.
func (p Project) Archive(now time.Time) (Project, error) {
	return p.transition(StatusArchived, now)
}

func (p Project) transition(target Status, now time.Time) (Project, error) {
	current := p.status
	if current == target {
		return p, nil
	}
	allowed, ok := validTransitions[current]
	if !ok {
		return Project{}, fmt.Errorf("project: no transitions defined for status %q", current)
	}
	if !allowed[target] {
		return Project{}, fmt.Errorf("project: cannot transition from %q to %q", current, target)
	}
	next := p
	next.status = target
	next.updatedAt = now.UTC()
	return next, nil
}

func (p Project) Record() Record {
	return Record{
		ID:            p.id,
		LocationID:    p.locationID,
		Root:          p.root,
		UserID:        p.userID,
		Title:         p.title,
		Status:        p.status,
		PlanMode:      p.planMode,
		Customization: p.customization,
		CreatedAt:     p.createdAt,
		UpdatedAt:     p.updatedAt,
	}
}
