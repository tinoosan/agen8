package contextlink

import (
	"context"
	"fmt"
	"time"
)

type ID string
type NodeType string
type EdgeType string

const (
	EdgeTypeBlockedBy   EdgeType = "blocked_by"
	EdgeTypeResolvedBy  EdgeType = "resolved_by"
	EdgeTypeCompletedBy EdgeType = "completed_by"
	EdgeTypeServes      EdgeType = "serves"
	EdgeTypeInformedBy  EdgeType = "informed_by"
	EdgeTypeProduced    EdgeType = "produced"
	EdgeTypeMadeDuring  EdgeType = "made_during"
	EdgeTypeSpawned     EdgeType = "spawned"
	EdgeTypeChildOf     EdgeType = "child_of"
	EdgeTypeRelatesTo   EdgeType = "relates_to"
)

var validEdgeTypes = map[EdgeType]struct{}{
	EdgeTypeBlockedBy:   {},
	EdgeTypeResolvedBy:  {},
	EdgeTypeCompletedBy: {},
	EdgeTypeServes:      {},
	EdgeTypeInformedBy:  {},
	EdgeTypeProduced:    {},
	EdgeTypeMadeDuring:  {},
	EdgeTypeSpawned:     {},
	EdgeTypeChildOf:     {},
	EdgeTypeRelatesTo:   {},
}

type NodeRef struct {
	Type NodeType
	ID   string
}

type Link struct {
	ID         ID
	Source     NodeRef
	Target     NodeRef
	EdgeType   EdgeType
	Confidence float64
	Metadata   map[string]string
	CreatedAt  time.Time
	CreatedBy  string
}

func ValidEdgeType(edgeType EdgeType) bool {
	_, ok := validEdgeTypes[edgeType]
	return ok
}

func (l Link) Validate() error {
	if l.Source.Type == "" {
		return fmt.Errorf("context link: source type is required")
	}
	if l.Source.ID == "" {
		return fmt.Errorf("context link: source id is required")
	}
	if l.Target.Type == "" {
		return fmt.Errorf("context link: target type is required")
	}
	if l.Target.ID == "" {
		return fmt.Errorf("context link: target id is required")
	}
	if l.Source.Type == l.Target.Type && l.Source.ID == l.Target.ID {
		return fmt.Errorf("context link: source and target must be different nodes")
	}
	if l.EdgeType == "" {
		return fmt.Errorf("context link: edge type is required")
	}
	if !ValidEdgeType(l.EdgeType) {
		return fmt.Errorf("context link: unknown edge type %q", l.EdgeType)
	}
	if l.Confidence < 0 || l.Confidence > 1 {
		return fmt.Errorf("context link: confidence must be in range [0.0, 1.0], got %f", l.Confidence)
	}
	return nil
}

type Reader interface {
	FindByID(ctx context.Context, id ID) (Link, error)
	FindBySource(ctx context.Context, source NodeRef) ([]Link, error)
	FindByTarget(ctx context.Context, target NodeRef) ([]Link, error)
	FindBetween(ctx context.Context, source NodeRef, target NodeRef) ([]Link, error)
	FindByEdgeType(ctx context.Context, edgeType EdgeType, limit int) ([]Link, error)
}

type Writer interface {
	Save(ctx context.Context, link Link) error
	Replace(ctx context.Context, link Link) error
}

type Deleter interface {
	Delete(ctx context.Context, id ID) error
	DeleteLinksForEntity(ctx context.Context, ref NodeRef) error
}

type Repository interface {
	Reader
	Writer
	Deleter
}

func LinkedEntities(ctx context.Context, reader Reader, ref NodeRef) ([]Link, error) {
	if reader == nil {
		return nil, fmt.Errorf("contextlink: reader is required")
	}
	if ref.Type == "" {
		return nil, fmt.Errorf("contextlink: entity type must not be empty")
	}
	if ref.ID == "" {
		return nil, fmt.Errorf("contextlink: entity id must not be empty")
	}
	asSource, err := reader.FindBySource(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("linked entities source %s/%s: %w", ref.Type, ref.ID, err)
	}
	asTarget, err := reader.FindByTarget(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("linked entities target %s/%s: %w", ref.Type, ref.ID, err)
	}
	seen := make(map[ID]struct{}, len(asSource)+len(asTarget))
	out := make([]Link, 0, len(asSource)+len(asTarget))
	for _, link := range append(asSource, asTarget...) {
		if _, ok := seen[link.ID]; ok {
			continue
		}
		seen[link.ID] = struct{}{}
		out = append(out, link)
	}
	return out, nil
}
