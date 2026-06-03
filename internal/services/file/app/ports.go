package app

import (
	"context"

	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type ProjectLoader interface {
	GetProject(ctx context.Context, projectID types.ProjectID) (projectdomain.Project, error)
	ListProjects(ctx context.Context, filter projectdomain.Filter) ([]projectdomain.Project, error)
}
