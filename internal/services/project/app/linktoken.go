package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
)

// LinkTokenIssuer mints wlt_ link tokens that bind a user to a project. The
// project service owns the authorization rule — only a project's owner may bind
// it — while the token's lifecycle (minting, hashing, storage) lives in the auth
// service. The project domain reaches that capability through this port so it
// never imports the auth packages; every identifier crossing the boundary is an
// opaque string, translated to typed ids by the adapter in the composition root.
type LinkTokenIssuer interface {
	IssueLinkToken(ctx context.Context, req LinkTokenRequest) (LinkTokenIssued, error)
}

// LinkTokenRequest is the issuance input handed to the issuer once the project
// service has verified the caller owns the named project.
type LinkTokenRequest struct {
	UserID      string
	ProjectID   string
	WorkspaceID string
	Label       string
	ExpiresAt   *time.Time
}

// LinkTokenIssued is the minted token plus its binding. Token is the raw wlt_
// secret, returned exactly once at mint time; everything else is safe to display
// or persist.
type LinkTokenIssued struct {
	ID          string
	Prefix      string
	Token       string
	UserID      string
	ProjectID   string
	WorkspaceID string
	Label       string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

// CreateLinkTokenInput is the project-service entry point for minting a link
// token. The caller identity is taken from context (the authenticated user), not
// from input, so a token can only ever bind its owner to a project they own.
type CreateLinkTokenInput struct {
	ProjectID types.ProjectID
	Label     string
	ExpiresAt *time.Time
}

// CreateLinkToken mints a wlt_ link token bound to (caller, project). It enforces
// project ownership before delegating to the issuer: the caller must be the
// authenticated owner of the target project. This is the server-side, unspoofable
// half of the binding — an MCP caller can never assert a project it was not
// granted, because the token is minted here only after an ownership check.
func (s *Service) CreateLinkToken(ctx context.Context, input CreateLinkTokenInput) (LinkTokenIssued, error) {
	if s == nil {
		return LinkTokenIssued{}, fmt.Errorf("project service is nil")
	}
	if s.linkTokens == nil {
		return LinkTokenIssued{}, fmt.Errorf("project link token issuer is required")
	}
	c, err := s.resolveCaller(ctx)
	if err != nil {
		return LinkTokenIssued{}, err
	}
	if c.UserID == "" {
		return LinkTokenIssued{}, fmt.Errorf("link token requires an authenticated user")
	}
	projectID := cleanProjectID(input.ProjectID)
	if projectID == "" {
		return LinkTokenIssued{}, fmt.Errorf("project id is required")
	}
	proj, err := s.GetProject(ctx, projectID)
	if err != nil {
		return LinkTokenIssued{}, err
	}
	if err := requireOwnedProject(c, proj); err != nil {
		return LinkTokenIssued{}, err
	}
	issued, err := s.linkTokens.IssueLinkToken(ctx, LinkTokenRequest{
		UserID:    c.UserID,
		ProjectID: string(proj.ID()),
		Label:     strings.TrimSpace(input.Label),
		ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return LinkTokenIssued{}, fmt.Errorf("issue link token: %w", err)
	}
	return issued, nil
}
