package cluster

import (
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type ID string
type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
)

type Cluster struct {
	id        ID
	projectID types.ProjectID
	name      string
	status    Status
	createdAt time.Time
	updatedAt time.Time
}

type NewInput struct {
	ID        ID
	ProjectID types.ProjectID
	Name      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

func New(input NewInput) (Cluster, error) {
	id := ID(strings.TrimSpace(string(input.ID)))
	if id == "" {
		return Cluster{}, fmt.Errorf("cluster id is required")
	}
	projectID := types.ProjectID(strings.TrimSpace(string(input.ProjectID)))
	if projectID == "" {
		return Cluster{}, fmt.Errorf("project id is required")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Cluster{}, fmt.Errorf("cluster name is required")
	}
	status := input.Status
	if status == "" {
		status = StatusOpen
	}
	if status != StatusOpen && status != StatusClosed {
		return Cluster{}, fmt.Errorf("unsupported cluster status %q", status)
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		return Cluster{}, fmt.Errorf("cluster created at is required")
	}
	updatedAt := input.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	return Cluster{
		id:        id,
		projectID: projectID,
		name:      name,
		status:    status,
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

func Wrap(record Record) (Cluster, error) {
	return New(NewInput{
		ID:        record.ID,
		ProjectID: record.ProjectID,
		Name:      record.Name,
		Status:    record.Status,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	})
}

func (c Cluster) ID() ID                     { return c.id }
func (c Cluster) ProjectID() types.ProjectID { return c.projectID }
func (c Cluster) Name() string               { return c.name }
func (c Cluster) Status() Status             { return c.status }
func (c Cluster) CreatedAt() time.Time       { return c.createdAt }
func (c Cluster) UpdatedAt() time.Time       { return c.updatedAt }
func (c Cluster) Record() Record {
	return Record{
		ID:        c.id,
		ProjectID: c.projectID,
		Name:      c.name,
		Status:    c.status,
		CreatedAt: c.createdAt,
		UpdatedAt: c.updatedAt,
	}
}
