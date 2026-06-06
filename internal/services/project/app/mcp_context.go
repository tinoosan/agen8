package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
)

type RegisterMCPContextInput struct {
	Token string
	// BoundProjectID is the project bound to the session server-side from its
	// link token (wlt_). It is authoritative and unspoofable: a caller cannot
	// set it, only the daemon can, from a validated token. When present it wins
	// over the caller-asserted ProjectID below.
	BoundProjectID   string
	UserID           string
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	DisplayName      string
	HarnessKind      string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
	Model            string
	Effort           string
	PermissionMode   string
	ConfigRef        string
}

type RegisterMCPContextResult struct {
	ProjectID        string
	ProjectRoot      string
	LocationID       string
	MemberID         string
	DisplayName      string
	MemberType       string
	ChannelID        string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
	Token            string
	URL              string
	MCPServers       []string
}

type ResolveMCPContextInput struct {
	Token            string
	UserID           string
	ProjectID        string
	HarnessKind      string
	SessionID        string
	ThreadID         string
	NativeSessionRef string
}

func (s *Service) RegisterMCPContext(ctx context.Context, input RegisterMCPContextInput) (RegisterMCPContextResult, error) {
	if s == nil {
		return RegisterMCPContextResult{}, fmt.Errorf("project service is nil")
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		return RegisterMCPContextResult{}, fmt.Errorf("mcp token is required")
	}
	harnessKind := strings.TrimSpace(input.HarnessKind)
	if harnessKind == "" {
		harnessKind = "unknown"
	}
	locationID := types.LocationID(strings.TrimSpace(input.LocationID))
	if locationID == "" {
		locationID = "local"
	}
	// Project identity in strict priority order (see docs: identity resolution).
	// 1. The link-token binding carried by the session — authoritative.
	projectID := types.ProjectID(strings.TrimSpace(input.BoundProjectID))
	if projectID == "" {
		// 2. An explicit caller-asserted id from a user-scoped token.
		projectID = types.ProjectID(strings.TrimSpace(input.ProjectID))
	}
	root := strings.TrimSpace(input.ProjectRoot)
	if projectID == "" {
		// 3. Path-hash fallback — last resort for unmarked and legacy folders.
		if root == "" {
			return RegisterMCPContextResult{}, fmt.Errorf("no project binding: marker, project_id, or project_root required")
		}
		projectID = ProjectIDForLocationRoot(locationID, root)
	}
	userID := userIDForMCPToken(token, input.UserID)
	ctx = caller.ContextWithCaller(ctx, caller.Caller{UserID: userID})
	loadedProject, err := s.GetProject(ctx, projectID)
	if err != nil {
		if !errors.Is(err, project.ErrNotFound) {
			return RegisterMCPContextResult{}, fmt.Errorf("load project: %w", err)
		}
		if root == "" {
			return RegisterMCPContextResult{}, fmt.Errorf("project %s not found; project_root is required to create it", projectID)
		}
		title := filepath.Base(filepath.Clean(root))
		loadedProject, err = s.CreateProject(ctx, CreateProjectInput{
			LocationID: locationID,
			Root:       root,
			Title:      title,
			Status:     project.StatusOpen,
		})
		if err != nil {
			return RegisterMCPContextResult{}, fmt.Errorf("create project: %w", err)
		}
	}
	loadedProject, err = s.rehomeLegacyLocalProject(ctx, loadedProject, userID)
	if err != nil {
		return RegisterMCPContextResult{}, err
	}
	projectID = loadedProject.ID()
	root = strings.TrimSpace(loadedProject.Root())
	locationID = loadedProject.LocationID()
	// Record the folder as a Workspace of this project — metadata, never identity.
	// This happens in every resolution path (bound, explicit, fallback): cwd tells
	// us where a workspace is, not which project it is.
	if _, err := s.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		ProjectID:  string(projectID),
		UserID:     userID,
		LocationID: string(locationID),
		Root:       root,
	}); err != nil {
		return RegisterMCPContextResult{}, fmt.Errorf("record workspace: %w", err)
	}
	nativeRef := nativeRefForRegister(input)
	if nativeRef == "" {
		nativeRef = "token:" + token
	}
	existing, err := s.findActiveMemberByNativeRef(ctx, string(projectID), harnessKind, nativeRef)
	if err != nil {
		return RegisterMCPContextResult{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if existing.ID == "" {
		memberType, err := s.nextRegisteredMemberType(ctx, string(projectID))
		if err != nil {
			return RegisterMCPContextResult{}, err
		}
		memberID := deterministicMemberID(projectID, harnessKind, nativeRef)
		if displayName == "" {
			displayName = harnessKind
		}
		existing, err = s.UpsertExternalHarnessMember(ctx, UpsertExternalHarnessMemberParams{
			ID:               memberID,
			UserID:           userID,
			ProjectID:        string(projectID),
			NativeSessionRef: nativeRef,
			DisplayName:      displayName,
			MemberType:       memberType,
			HarnessKind:      harnessKind,
			Model:            strings.TrimSpace(input.Model),
			Effort:           strings.TrimSpace(input.Effort),
			PermissionMode:   strings.TrimSpace(input.PermissionMode),
			ConfigRef:        strings.TrimSpace(input.ConfigRef),
		})
		if err != nil {
			return RegisterMCPContextResult{}, fmt.Errorf("register member: %w", err)
		}
	} else if displayName != "" && displayName != strings.TrimSpace(existing.DisplayName) {
		existing.DisplayName = displayName
		existing.UpdatedAt = s.clock.Now().UTC()
		if err := s.members.Update(ctx, existing); err != nil {
			return RegisterMCPContextResult{}, fmt.Errorf("update registered member display name: %w", err)
		}
		existing = s.withResolvedPermissionMode(existing)
	}
	if existing.ID != "" && strings.TrimSpace(existing.UserID) != userID {
		if strings.TrimSpace(existing.UserID) != "local" || userID == "local" {
			return RegisterMCPContextResult{}, fmt.Errorf("registered member %s belongs to a different user", existing.ID)
		}
		existing.UserID = userID
		existing.UpdatedAt = s.clock.Now().UTC()
		if err := s.members.Update(ctx, existing); err != nil {
			return RegisterMCPContextResult{}, fmt.Errorf("rehome registered member to mcp token user: %w", err)
		}
		existing = s.withResolvedPermissionMode(existing)
	}
	return RegisterMCPContextResult{
		ProjectID:        string(projectID),
		ProjectRoot:      root,
		LocationID:       string(locationID),
		MemberID:         string(existing.ID),
		DisplayName:      strings.TrimSpace(existing.DisplayName),
		MemberType:       strings.TrimSpace(existing.MemberType),
		ChannelID:        strings.TrimSpace(existing.ChannelID),
		SessionID:        strings.TrimSpace(input.SessionID),
		ThreadID:         strings.TrimSpace(input.ThreadID),
		NativeSessionRef: nativeRef,
		Token:            token,
		URL:              "",
		MCPServers:       []string{"agen8"},
	}, nil
}

func (s *Service) rehomeLegacyLocalProject(ctx context.Context, loaded project.Project, userID string) (project.Project, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || userID == "local" {
		return loaded, nil
	}
	currentUserID := strings.TrimSpace(loaded.UserID())
	if currentUserID == "" || currentUserID == userID {
		return loaded, nil
	}
	if currentUserID != "local" {
		return project.Project{}, fmt.Errorf("project %s belongs to a different user", loaded.ID())
	}
	record := loaded.Record()
	record.UserID = userID
	record.UpdatedAt = s.clock.Now().UTC()
	saved, err := s.projects.Save(ctx, record)
	if err != nil {
		return project.Project{}, fmt.Errorf("rehome project to mcp token user: %w", err)
	}
	rehomed, err := project.Wrap(saved)
	if err != nil {
		return project.Project{}, fmt.Errorf("wrap rehomed project: %w", err)
	}
	return rehomed, nil
}

func (s *Service) ResolveMCPContext(ctx context.Context, input ResolveMCPContextInput) (member.Record, error) {
	if s == nil {
		return member.Record{}, fmt.Errorf("project service is nil")
	}
	harnessKind := strings.TrimSpace(input.HarnessKind)
	nativeRef := nativeRefForResolve(input)
	if nativeRef == "" {
		return member.Record{}, member.ErrNotFound
	}
	userID := userIDForMCPToken(input.Token, input.UserID)
	ctx = caller.ContextWithCaller(ctx, caller.Caller{UserID: userID})
	filter := member.Filter{
		UserID:           userID,
		HarnessKind:      harnessKind,
		NativeSessionRef: nativeRef,
		LifecycleState:   member.LifecycleActive,
		Limit:            2,
	}
	if projectID := strings.TrimSpace(input.ProjectID); projectID != "" {
		filter.ProjectID = projectID
	}
	members, err := s.members.List(ctx, filter)
	if err != nil {
		return member.Record{}, fmt.Errorf("resolve mcp context member: %w", err)
	}
	if len(members) == 0 {
		return member.Record{}, member.ErrNotFound
	}
	if len(members) > 1 {
		return member.Record{}, fmt.Errorf("mcp context resolves to multiple members")
	}
	return s.withResolvedPermissionMode(members[0]), nil
}

func (s *Service) findActiveMemberByNativeRef(ctx context.Context, projectID, harnessKind, nativeRef string) (member.Record, error) {
	members, err := s.members.List(ctx, member.Filter{
		ProjectID:        strings.TrimSpace(projectID),
		HarnessKind:      strings.TrimSpace(harnessKind),
		NativeSessionRef: strings.TrimSpace(nativeRef),
		LifecycleState:   member.LifecycleActive,
		Limit:            2,
	})
	if err != nil {
		return member.Record{}, fmt.Errorf("find registered member: %w", err)
	}
	if len(members) == 0 {
		return member.Record{}, nil
	}
	if len(members) > 1 {
		return member.Record{}, fmt.Errorf("multiple active members match native session")
	}
	return s.withResolvedPermissionMode(members[0]), nil
}

func (s *Service) nextRegisteredMemberType(ctx context.Context, projectID string) (string, error) {
	if _, err := s.members.List(ctx, member.Filter{
		ProjectID:      strings.TrimSpace(projectID),
		LifecycleState: member.LifecycleActive,
		Limit:          1,
	}); err != nil {
		return "", fmt.Errorf("list project members: %w", err)
	}
	return member.TypeCoordinator, nil
}

func nativeRefForRegister(input RegisterMCPContextInput) string {
	if ref := strings.TrimSpace(input.NativeSessionRef); ref != "" {
		return ref
	}
	if ref := strings.TrimSpace(input.ThreadID); ref != "" {
		return ref
	}
	return strings.TrimSpace(input.SessionID)
}

func nativeRefForResolve(input ResolveMCPContextInput) string {
	if ref := strings.TrimSpace(input.NativeSessionRef); ref != "" {
		return ref
	}
	if ref := strings.TrimSpace(input.ThreadID); ref != "" {
		return ref
	}
	return strings.TrimSpace(input.SessionID)
}

func userIDForMCPToken(token string, explicitUserID string) string {
	if userID := strings.TrimSpace(explicitUserID); userID != "" {
		return userID
	}
	return "local"
}

func deterministicMemberID(projectID types.ProjectID, harnessKind, nativeRef string) member.ID {
	sum := sha1.Sum([]byte(string(projectID) + "\x00" + strings.TrimSpace(harnessKind) + "\x00" + strings.TrimSpace(nativeRef)))
	return member.ID("member-" + hex.EncodeToString(sum[:])[:16])
}
