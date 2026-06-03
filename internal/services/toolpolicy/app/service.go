package app

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/toolpolicy/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/membertype"
)

// AuthorizeParams holds the inputs for tool authorization.
type AuthorizeParams struct {
	SpaceID      string
	MemberType   membertype.MemberType
	MemberCount  int
	HasReviewer  bool
	AllowedTools []string
}

// AuthorizeResult holds the outcome of tool authorization.
type AuthorizeResult struct {
	Allowed []string
	Removed []string
}

// SystemToolsParams holds the inputs for querying system tools.
type SystemToolsParams struct {
	MemberType  membertype.MemberType
	MemberCount int
	HasReviewer bool
}

// DefaultsResult holds the default tool lists by member type.
type DefaultsResult struct {
	WorkerTools            []string
	CoordinatorBase        []string
	CoordinatorWithWorkers []string
}

// Service provides tool policy operations over the domain aggregate.
type Service struct{}

// NewService creates a new tool policy application service.
func NewService() *Service {
	return &Service{}
}

// Authorize builds a RoleToolPolicy from the given context and authorizes
// the requested tools, returning which tools are allowed and which were removed.
func (s *Service) Authorize(_ context.Context, params AuthorizeParams) (AuthorizeResult, error) {
	policy := domain.NewRoleToolPolicy(domain.RoleToolContext{
		MemberType:  params.MemberType,
		SpaceID:     params.SpaceID,
		MemberCount: params.MemberCount,
		HasReviewer: params.HasReviewer,
	})
	result := policy.Authorize(params.AllowedTools)
	return AuthorizeResult{
		Allowed: result.Allowed,
		Removed: result.Removed,
	}, nil
}

// SystemTools returns the system tools for a role with the given context.
func (s *Service) SystemTools(_ context.Context, params SystemToolsParams) ([]string, error) {
	if params.MemberType == nil {
		return nil, fmt.Errorf("MemberType is required")
	}
	policy := domain.NewRoleToolPolicy(domain.RoleToolContext{
		MemberType:  params.MemberType,
		MemberCount: params.MemberCount,
		HasReviewer: params.HasReviewer,
	})
	return policy.SystemTools(), nil
}

// Defaults returns the default tool lists for each member type.
func (s *Service) Defaults(_ context.Context) DefaultsResult {
	return DefaultsResult{
		WorkerTools:            domain.DefaultWorkerTools(),
		CoordinatorBase:        domain.CoordinatorBaseToolNames(),
		CoordinatorWithWorkers: domain.CoordinatorWithWorkersToolNames(),
	}
}
