package app

import (
	"context"
	"crypto/sha1" // #nosec G505 -- used only for stable legacy-compatible identifiers, not cryptography.
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
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
	ProjectID         string
	ProjectRoot       string
	LocationID        string
	MemberID          string
	DisplayName       string
	MemberType        string
	ChannelID         string
	SessionID         string
	ThreadID          string
	NativeSessionRef  string
	MCPServers        []string
	AlreadyRegistered bool
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
	requestedRoot := strings.TrimSpace(input.ProjectRoot)
	identityRoot := requestedRoot
	if projectID == "" {
		// 3. Path-hash fallback — last resort for unmarked and legacy folders.
		if identityRoot == "" {
			return RegisterMCPContextResult{}, fmt.Errorf("no project binding: marker, project_id, or project_root required")
		}
		if canonicalRoot, ok := canonicalGitWorktreeRoot(identityRoot, locationID); ok {
			identityRoot = canonicalRoot
		}
		projectID = ProjectIDForLocationRoot(locationID, identityRoot)
	}
	userID := userIDForMCPToken(token, input.UserID)
	ctx = caller.ContextWithCaller(ctx, caller.Caller{UserID: userID})
	loadedProject, err := s.loadProject(ctx, projectID)
	if err != nil {
		if !errors.Is(err, project.ErrNotFound) {
			return RegisterMCPContextResult{}, fmt.Errorf("load project: %w", err)
		}
		if identityRoot == "" {
			return RegisterMCPContextResult{}, fmt.Errorf("project %s not found; project_root is required to create it", projectID)
		}
		title := filepath.Base(filepath.Clean(identityRoot))
		loadedProject, err = s.CreateProject(ctx, CreateProjectInput{
			LocationID: locationID,
			Root:       identityRoot,
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
	projectRoot := strings.TrimSpace(loadedProject.Root())
	locationID = loadedProject.LocationID()
	workspaceRoot := trustedWorkspaceRoot(requestedRoot, projectRoot, locationID)
	// Record the folder as a Workspace of this project — metadata, never identity.
	// This happens in every resolution path (bound, explicit, fallback): cwd tells
	// us where a workspace is, not which project it is.
	boundWorkspace, err := s.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		ProjectID:  string(projectID),
		UserID:     userID,
		LocationID: string(locationID),
		Root:       workspaceRoot,
	})
	if err != nil {
		return RegisterMCPContextResult{}, fmt.Errorf("record workspace: %w", err)
	}
	workspaceID := strings.TrimSpace(string(boundWorkspace.ID))
	nativeRef := nativeRefForRegister(input)
	if nativeRef == "" {
		nativeRef = "token:" + token
	}
	existing, err := s.findActiveMemberByNativeRef(ctx, string(projectID), nativeRef)
	if err != nil {
		return RegisterMCPContextResult{}, err
	}
	reusedExisting := existing.ID != ""
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
			WorkspaceID:      workspaceID,
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
	if strings.TrimSpace(existing.WorkspaceID) != workspaceID {
		existing.WorkspaceID = workspaceID
		existing.UpdatedAt = s.clock.Now().UTC()
		if err := s.members.Update(ctx, existing); err != nil {
			return RegisterMCPContextResult{}, fmt.Errorf("bind registered member to workspace: %w", err)
		}
	}
	return RegisterMCPContextResult{
		ProjectID:         string(projectID),
		ProjectRoot:       workspaceRoot,
		LocationID:        string(locationID),
		MemberID:          string(existing.ID),
		DisplayName:       strings.TrimSpace(existing.DisplayName),
		MemberType:        strings.TrimSpace(existing.MemberType),
		ChannelID:         strings.TrimSpace(existing.ChannelID),
		SessionID:         strings.TrimSpace(input.SessionID),
		ThreadID:          strings.TrimSpace(input.ThreadID),
		NativeSessionRef:  nativeRef,
		MCPServers:        []string{"agen8"},
		AlreadyRegistered: reusedExisting,
	}, nil
}

func canonicalGitWorktreeRoot(root string, locationID types.LocationID) (string, bool) {
	if strings.TrimSpace(string(locationID)) != "" && locationID != "local" {
		return "", false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}
	gitDir, ok := gitOutput(root, "rev-parse", "--absolute-git-dir")
	if !ok {
		return "", false
	}
	commonDir, ok := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok {
		return "", false
	}
	gitDir = filepath.Clean(gitDir)
	commonDir = filepath.Clean(commonDir)
	if gitDir == "" || commonDir == "" || filepath.Clean(gitDir) == filepath.Clean(commonDir) {
		return "", false
	}
	list, ok := gitOutput(root, "worktree", "list", "--porcelain")
	if !ok {
		return "", false
	}
	for _, line := range strings.Split(list, "\n") {
		path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if path != line && path != "" {
			return filepath.Clean(path), true
		}
	}
	return "", false
}

func gitOutput(root string, args ...string) (string, bool) {
	cmdArgs := append([]string{"-C", root}, args...)
	out, err := exec.Command("git", cmdArgs...).Output() // #nosec G204 -- fixed binary with argv-only arguments; no shell interpolation.
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
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
		// No Limit on purpose. We must see every candidate to tell a same-session
		// harness-label fork (collapsible) apart from a genuine cross-project match
		// (must fail loudly). A bound limit could hide a second project's member
		// behind the first project's rows and turn a loud ambiguity into a silent,
		// possibly wrong, pick. The match is already scoped to one user and one exact
		// native ref, so the candidate set is inherently small.
	}
	if projectID := strings.TrimSpace(input.ProjectID); projectID != "" {
		filter.ProjectID = projectID
	}
	members, err := s.members.List(ctx, filter)
	if err != nil {
		return member.Record{}, fmt.Errorf("resolve mcp context member: %w", err)
	}
	resolved, err := collapseSessionMembers(members)
	if err != nil {
		return member.Record{}, err
	}
	return s.withResolvedPermissionMode(resolved), nil
}

// findActiveMemberByNativeRef finds the active member that owns a native session
// ref within one project, ignoring harness label. Harness kind seeds a member's id
// (deterministicMemberID) but does not change which session a ref belongs to, so a
// label that drifted between registrations - "claude" then "claude-cli" - must still
// resolve back to the same member instead of forking a new one. The lookup is already
// scoped to one project, so any duplicates are a same-session fork and collapse to the
// original (earliest-registered) member. A zero-id record means no match: the caller
// then creates a fresh member.
func (s *Service) findActiveMemberByNativeRef(ctx context.Context, projectID, nativeRef string) (member.Record, error) {
	members, err := s.members.List(ctx, member.Filter{
		ProjectID:        strings.TrimSpace(projectID),
		NativeSessionRef: strings.TrimSpace(nativeRef),
		LifecycleState:   member.LifecycleActive,
	})
	if err != nil {
		return member.Record{}, fmt.Errorf("find registered member: %w", err)
	}
	if len(members) == 0 {
		return member.Record{}, nil
	}
	resolved, err := collapseSessionMembers(members)
	if err != nil {
		return member.Record{}, err
	}
	return s.withResolvedPermissionMode(resolved), nil
}

// collapseSessionMembers turns the active members that matched one
// (user, native_session_ref) lookup into the single member that owns the session.
//
// A native session ref belongs to one session, and one session is one member. But
// harness identity is part of a member's id (deterministicMemberID), so a session
// whose harness label drifted between registrations forks into two member rows for
// the same human session. Both rows carry the same project and native ref; only the
// cosmetic label differs. Collapsing them to one member is correct - they are the
// same actor, not a wrong one.
//
// The collapse is bounded to a single project on purpose. When candidates span two
// projects we cannot know which the caller means - the lookup was not project-scoped
// (an api-key session carries no bound project) and the native ref happens to be
// shared - so we keep failing loudly rather than guess an actor across a project
// boundary. An empty input is member.ErrNotFound so callers can treat it as "no
// member yet", which the daemon maps to a member-less (pre-registration) session.
func collapseSessionMembers(members []member.Record) (member.Record, error) {
	if len(members) == 0 {
		return member.Record{}, member.ErrNotFound
	}
	winner := members[0]
	for _, candidate := range members[1:] {
		if strings.TrimSpace(candidate.ProjectID) != strings.TrimSpace(winner.ProjectID) {
			return member.Record{}, fmt.Errorf("mcp context resolves to multiple members")
		}
		if preferRegisteredMember(candidate, winner) {
			winner = candidate
		}
	}
	return winner, nil
}

// preferRegisteredMember reports whether candidate should win over current when both
// describe the same session. The EARLIEST-registered member wins, because it is the
// original identity the session created and any work attributed before a harness-label
// fork lives on it: until this fix a fork blocked every actor call, so the later member
// never claimed a task. Collapsing to the original lets the session resume as the
// claimant of its own work rather than orphaning it under an id it can no longer act as.
// The member id breaks ties so the choice stays deterministic when timestamps match (as
// they do under a fixed test clock, or for two registrations in the same instant).
func preferRegisteredMember(candidate, current member.Record) bool {
	if candidate.RegisteredAt.Before(current.RegisteredAt) {
		return true
	}
	if candidate.RegisteredAt.Equal(current.RegisteredAt) {
		return string(candidate.ID) < string(current.ID)
	}
	return false
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

func trustedWorkspaceRoot(requestedRoot, projectRoot string, locationID types.LocationID) string {
	requestedRoot = strings.TrimSpace(requestedRoot)
	projectRoot = strings.TrimSpace(projectRoot)
	if requestedRoot == "" || projectRoot == "" {
		return projectRoot
	}
	if workspaceRootMatchesProjectRoot(requestedRoot, projectRoot, locationID) {
		return requestedRoot
	}
	return projectRoot
}

func workspaceRootMatchesProjectRoot(requestedRoot, projectRoot string, locationID types.LocationID) bool {
	if strings.TrimSpace(string(locationID)) != "" && locationID != "local" {
		return strings.TrimSpace(requestedRoot) == strings.TrimSpace(projectRoot)
	}
	requestedClean := filepath.Clean(strings.TrimSpace(requestedRoot))
	projectClean := filepath.Clean(strings.TrimSpace(projectRoot))
	if requestedClean == projectClean {
		return true
	}
	requestedCommon, requestedOK := gitOutput(requestedClean, "rev-parse", "--path-format=absolute", "--git-common-dir")
	projectCommon, projectOK := gitOutput(projectClean, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !requestedOK || !projectOK {
		return false
	}
	return filepath.Clean(requestedCommon) == filepath.Clean(projectCommon)
}

func userIDForMCPToken(token string, explicitUserID string) string {
	if userID := strings.TrimSpace(explicitUserID); userID != "" {
		return userID
	}
	return "local"
}

func deterministicMemberID(projectID types.ProjectID, harnessKind, nativeRef string) member.ID {
	// #nosec G401 -- durable compatibility identifier; not used for cryptographic security.
	sum := sha1.Sum([]byte(string(projectID) + "\x00" + strings.TrimSpace(harnessKind) + "\x00" + strings.TrimSpace(nativeRef)))
	return member.ID("member-" + hex.EncodeToString(sum[:])[:16])
}
