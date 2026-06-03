package cluster

import (
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type SpaceRef struct {
	clusterID ID
	spaceID   spacedomain.SpaceID
	sortOrder int
	pinned    bool
}

type NewSpaceRefInput struct {
	ClusterID ID
	SpaceID   spacedomain.SpaceID
	SortOrder int
	Pinned    bool
}

func NewSpaceRef(input NewSpaceRefInput) (SpaceRef, error) {
	clusterID := ID(strings.TrimSpace(string(input.ClusterID)))
	if clusterID == "" {
		return SpaceRef{}, fmt.Errorf("cluster id is required")
	}
	spaceID := spacedomain.SpaceID(strings.TrimSpace(string(input.SpaceID)))
	if spaceID == "" {
		return SpaceRef{}, fmt.Errorf("space id is required")
	}
	return SpaceRef{
		clusterID: clusterID,
		spaceID:   spaceID,
		sortOrder: input.SortOrder,
		pinned:    input.Pinned,
	}, nil
}

func WrapSpaceRef(record SpaceRefRecord) (SpaceRef, error) {
	return NewSpaceRef(NewSpaceRefInput{
		ClusterID: record.ClusterID,
		SpaceID:   record.SpaceID,
		SortOrder: record.SortOrder,
		Pinned:    record.Pinned,
	})
}

func (r SpaceRef) ClusterID() ID                { return r.clusterID }
func (r SpaceRef) SpaceID() spacedomain.SpaceID { return r.spaceID }
func (r SpaceRef) SortOrder() int               { return r.sortOrder }
func (r SpaceRef) Pinned() bool                 { return r.pinned }
func (r SpaceRef) Record() SpaceRefRecord {
	return SpaceRefRecord{
		ClusterID: r.clusterID,
		SpaceID:   r.spaceID,
		SortOrder: r.sortOrder,
		Pinned:    r.pinned,
	}
}
