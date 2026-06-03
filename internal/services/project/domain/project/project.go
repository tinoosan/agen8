package project

import (
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusArchived Status = "archived"
)

type Project struct {
	id         types.ProjectID
	locationID types.LocationID
	root       string
	title      string
	status     Status
	createdAt  time.Time
	updatedAt  time.Time
}

type NewInput struct {
	ID         types.ProjectID
	LocationID types.LocationID
	Root       string
	Title      string
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	if status != StatusOpen && status != StatusArchived {
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
		id:         id,
		locationID: locationID,
		root:       root,
		title:      strings.TrimSpace(input.Title),
		status:     status,
		createdAt:  createdAt.UTC(),
		updatedAt:  updatedAt.UTC(),
	}, nil
}

func Wrap(record Record) (Project, error) {
	return New(NewInput{
		ID:         record.ID,
		LocationID: record.LocationID,
		Root:       record.Root,
		Title:      record.Title,
		Status:     record.Status,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	})
}

func (p Project) ID() types.ProjectID          { return p.id }
func (p Project) LocationID() types.LocationID { return p.locationID }
func (p Project) Root() string                 { return p.root }
func (p Project) Title() string                { return p.title }
func (p Project) Status() Status               { return p.status }
func (p Project) CreatedAt() time.Time {
	return p.createdAt
}
func (p Project) UpdatedAt() time.Time {
	return p.updatedAt
}

func (p Project) Record() Record {
	return Record{
		ID:         p.id,
		LocationID: p.locationID,
		Root:       p.root,
		Title:      p.title,
		Status:     p.status,
		CreatedAt:  p.createdAt,
		UpdatedAt:  p.updatedAt,
	}
}
