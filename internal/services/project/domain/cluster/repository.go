package cluster

import (
	"context"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Record struct {
	ID        ID
	ProjectID types.ProjectID
	Name      string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SpaceRefRecord struct {
	ClusterID ID
	SpaceID   spacedomain.SpaceID
	SortOrder int
	Pinned    bool
}

type Filter struct {
	ProjectID types.ProjectID
	Status    Status
}

type Reader interface {
	List(ctx context.Context, filter Filter) ([]Record, error)
	ListSpaces(ctx context.Context, clusterID ID) ([]SpaceRefRecord, error)
}

type Writer interface {
	Save(ctx context.Context, cluster Record) (Record, error)
	SaveSpace(ctx context.Context, ref SpaceRefRecord) (SpaceRefRecord, error)
	RemoveSpace(ctx context.Context, clusterID ID, spaceID spacedomain.SpaceID) error
}

type Repository interface {
	Reader
	Writer
}
