package app

import (
	"context"

	"github.com/tinoosan/agen8/internal/core/types"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
)

type ProjectLoader interface {
	GetProject(ctx context.Context, projectID types.ProjectID) (projectdomain.Project, error)
	ListProjects(ctx context.Context, filter projectdomain.Filter) ([]projectdomain.Project, error)
	// ResolveRoot returns the project's effective filesystem root: the live
	// workspace root when one is known, otherwise the stored project.root. File
	// operations resolve against this so they follow a folder that was moved or
	// renamed on disk after the project was first linked.
	ResolveRoot(ctx context.Context, p projectdomain.Project) string
}
