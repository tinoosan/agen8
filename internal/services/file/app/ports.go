package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
)

type ProjectSnapshot struct {
	ID           types.ProjectID
	LocationID   types.LocationID
	Root         string
	ResolvedRoot string
}

func (p ProjectSnapshot) EffectiveRoot() string {
	if resolved := strings.TrimSpace(p.ResolvedRoot); resolved != "" {
		return resolved
	}
	return strings.TrimSpace(p.Root)
}

type ProjectFilter struct {
	Status string
	Limit  int
	Offset int
}

type ProjectLoader interface {
	GetProject(ctx context.Context, projectID types.ProjectID) (ProjectSnapshot, error)
	ListProjects(ctx context.Context, filter ProjectFilter) ([]ProjectSnapshot, error)
}
