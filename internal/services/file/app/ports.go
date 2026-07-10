package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
)

type ProjectSnapshot struct {
	ID         types.ProjectID
	LocationID types.LocationID
	Root       string
}

func (p ProjectSnapshot) EffectiveRoot() string {
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
