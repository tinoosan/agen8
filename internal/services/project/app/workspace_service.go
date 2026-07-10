package app

import (
	"context"
	"crypto/sha1" // #nosec G505 -- used only for stable legacy-compatible identifiers, not cryptography.
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
)

// UpsertWorkspaceParams records one place a project is linked: a
// (location, root, machine) triple owned by a project. ProjectID and Root are
// required; the rest default to the local single-machine case.
type UpsertWorkspaceParams struct {
	ProjectID  string
	UserID     string
	LocationID string
	Root       string
	Machine    string
}

// UpsertWorkspace records (or re-touches) the workspace a session is acting in.
// It is identity-stable: the same (project, location, root, machine) always maps
// to the same workspace id, so re-registration touches the existing row rather
// than creating duplicates. LinkedAt is preserved across touches; LastSeenAt and
// UpdatedAt advance to now.
func (s *Service) UpsertWorkspace(ctx context.Context, params UpsertWorkspaceParams) (workspace.Record, error) {
	if s == nil {
		return workspace.Record{}, fmt.Errorf("project service is nil")
	}
	projectID := strings.TrimSpace(params.ProjectID)
	if projectID == "" {
		return workspace.Record{}, fmt.Errorf("workspace project id is required")
	}
	root := strings.TrimSpace(params.Root)
	if root == "" {
		return workspace.Record{}, fmt.Errorf("workspace root is required")
	}
	locationID := strings.TrimSpace(params.LocationID)
	if locationID == "" {
		locationID = "local"
	}
	userID := strings.TrimSpace(params.UserID)
	if userID == "" {
		userID = "local"
	}
	machine := strings.TrimSpace(params.Machine)
	now := s.now()
	id := deterministicWorkspaceID(projectID, locationID, root, machine)

	existing, err := s.workspaces.Get(ctx, string(id))
	if err != nil {
		if !errors.Is(err, workspace.ErrNotFound) {
			return workspace.Record{}, fmt.Errorf("load workspace %s: %w", id, err)
		}
		created := workspace.WrapWorkspace(workspace.Record{
			ID:         id,
			ProjectID:  projectID,
			UserID:     userID,
			LocationID: locationID,
			Root:       root,
			Machine:    machine,
			LinkedAt:   now,
		}).Touch(now)
		if err := s.workspaces.Create(ctx, created.Inner()); err != nil {
			return workspace.Record{}, fmt.Errorf("create workspace %s: %w", id, err)
		}
		return created.Inner(), nil
	}

	touched := workspace.WrapWorkspace(existing).Touch(now)
	if err := s.workspaces.Update(ctx, touched.Inner()); err != nil {
		return workspace.Record{}, fmt.Errorf("update workspace %s: %w", id, err)
	}
	return touched.Inner(), nil
}

// ListWorkspaces returns the workspaces a project is linked from.
func (s *Service) ListWorkspaces(ctx context.Context, filter workspace.Filter) ([]workspace.Record, error) {
	if s == nil {
		return nil, fmt.Errorf("project service is nil")
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, fmt.Errorf("workspace limit and offset must be non-negative")
	}
	return s.workspaces.List(ctx, filter)
}

// ResolveBoundWorkspaceRoot returns the exact active workspace root bound to a
// registered member. Every ownership and location field must still match the
// project; a stale or corrupted binding fails closed instead of falling back to
// another session's most-recent workspace.
func (s *Service) ResolveBoundWorkspaceRoot(ctx context.Context, projectID, userID, locationID, workspaceID string) (string, error) {
	if s == nil || s.workspaces == nil {
		return "", fmt.Errorf("workspace repository is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return "", fmt.Errorf("workspace binding is required")
	}
	bound, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("load bound workspace %s: %w", workspaceID, err)
	}
	if strings.TrimSpace(bound.ProjectID) != strings.TrimSpace(projectID) {
		return "", fmt.Errorf("bound workspace does not belong to the session project")
	}
	if strings.TrimSpace(bound.UserID) != strings.TrimSpace(userID) {
		return "", fmt.Errorf("bound workspace does not belong to the session user")
	}
	if normalizeLocationID(bound.LocationID) != normalizeLocationID(locationID) {
		return "", fmt.Errorf("bound workspace does not belong to the project location")
	}
	if strings.TrimSpace(bound.LifecycleState) != workspace.LifecycleActive {
		return "", fmt.Errorf("bound workspace is not active")
	}
	root := strings.TrimSpace(bound.Root)
	if root == "" {
		return "", fmt.Errorf("bound workspace root is empty")
	}
	return root, nil
}

// deterministicWorkspaceID derives a stable workspace id from its identity tuple.
// It mirrors deterministicMemberID: same inputs, same id, so a re-link of the
// same folder on the same machine resolves to the same workspace row.
func deterministicWorkspaceID(projectID, locationID, root, machine string) workspace.ID {
	// #nosec G401 -- durable compatibility identifier; not used for cryptographic security.
	sum := sha1.Sum([]byte(strings.TrimSpace(projectID) + "\x00" +
		strings.TrimSpace(locationID) + "\x00" +
		strings.TrimSpace(root) + "\x00" +
		strings.TrimSpace(machine)))
	return workspace.ID("ws-" + hex.EncodeToString(sum[:])[:16])
}
