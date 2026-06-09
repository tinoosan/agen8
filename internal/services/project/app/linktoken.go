package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
)

// LinkTokenService is the project service's gateway to the wlt_ link token
// capability. The project service owns the authorization rule — only a
// project's owner may mint, list, or revoke its tokens — while the token's
// lifecycle (minting, hashing, storage) lives in the auth service. The project
// domain reaches that capability through this port so it never imports the auth
// packages; every identifier crossing the boundary is an opaque string,
// translated to typed ids by the adapter in the composition root.
type LinkTokenService interface {
	IssueLinkToken(ctx context.Context, req LinkTokenRequest) (LinkTokenIssued, error)
	// ListLinkTokens returns non-secret summaries of the tokens bound to a
	// project. The raw secret is never recoverable, so this is safe to surface.
	ListLinkTokens(ctx context.Context, projectID string) ([]LinkTokenSummary, error)
	// RevokeLinkToken marks a token revoked by id. The project service has
	// already verified the token belongs to a project the caller owns.
	RevokeLinkToken(ctx context.Context, tokenID string) error
}

// LinkTokenSummary is the displayable, non-secret view of a minted link token:
// enough to recognize and manage it (prefix, label, timestamps, state) but
// never the raw token or its hash. Active is the auth service's own
// clock-evaluated verdict; RevokedAt and ExpiresAt let a caller distinguish a
// revoked token from a merely expired one.
type LinkTokenSummary struct {
	ID          string
	Prefix      string
	ProjectID   string
	WorkspaceID string
	Label       string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	Active      bool
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

// ownedProjectForLinkToken resolves the caller and confirms they own the named
// project. It is the shared gate in front of every link-token operation: mint,
// list, and revoke all require the same proof, so an MCP caller can never read
// or revoke tokens for a project they were not granted.
func (s *Service) ownedProjectForLinkToken(ctx context.Context, projectID types.ProjectID) (project.Project, error) {
	if s == nil {
		return project.Project{}, fmt.Errorf("project service is nil")
	}
	if s.linkTokens == nil {
		return project.Project{}, fmt.Errorf("project link token service is required")
	}
	c, err := s.resolveCaller(ctx)
	if err != nil {
		return project.Project{}, err
	}
	if c.UserID == "" {
		return project.Project{}, fmt.Errorf("link token requires an authenticated user")
	}
	id := cleanProjectID(projectID)
	if id == "" {
		return project.Project{}, fmt.Errorf("project id is required")
	}
	proj, err := s.GetProject(ctx, id)
	if err != nil {
		return project.Project{}, err
	}
	if err := requireOwnedProject(c, proj); err != nil {
		return project.Project{}, err
	}
	return proj, nil
}

// ListLinkTokens returns the link tokens bound to a project the caller owns.
// Summaries carry no secret, so the only thing gated here is ownership: a
// non-owner cannot enumerate a project's tokens.
func (s *Service) ListLinkTokens(ctx context.Context, projectID types.ProjectID) ([]LinkTokenSummary, error) {
	proj, err := s.ownedProjectForLinkToken(ctx, projectID)
	if err != nil {
		return nil, err
	}
	summaries, err := s.linkTokens.ListLinkTokens(ctx, string(proj.ID()))
	if err != nil {
		return nil, fmt.Errorf("list link tokens: %w", err)
	}
	return summaries, nil
}

// RevokeLinkToken revokes one of a project's link tokens. Beyond the ownership
// gate, it re-derives the project's own token set and rejects a tokenID that is
// not in it — so a caller who owns project A can never revoke project B's token
// by guessing its id.
func (s *Service) RevokeLinkToken(ctx context.Context, projectID types.ProjectID, tokenID string) error {
	proj, err := s.ownedProjectForLinkToken(ctx, projectID)
	if err != nil {
		return err
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return fmt.Errorf("link token id is required")
	}
	summaries, err := s.linkTokens.ListLinkTokens(ctx, string(proj.ID()))
	if err != nil {
		return fmt.Errorf("list link tokens: %w", err)
	}
	owned := false
	for _, summary := range summaries {
		if summary.ID == tokenID {
			owned = true
			break
		}
	}
	if !owned {
		return fmt.Errorf("link token %q does not belong to project %q", tokenID, proj.ID())
	}
	if err := s.linkTokens.RevokeLinkToken(ctx, tokenID); err != nil {
		return fmt.Errorf("revoke link token: %w", err)
	}
	return nil
}
