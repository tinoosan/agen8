package app

import (
	"context"

	"github.com/tinoosan/agen8/internal/core/types"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
)

type ProjectLoader interface {
	GetProject(ctx context.Context, projectID types.ProjectID) (projectdomain.Project, error)
	ListProjects(ctx context.Context, filter projectdomain.Filter) ([]projectdomain.Project, error)
}
