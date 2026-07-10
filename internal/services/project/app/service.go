package app

import (
	"context"
	"crypto/sha1" // #nosec G505 -- used only for stable legacy-compatible identifiers, not cryptography.
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
)

type Service struct {
	projects   project.Repository
	members    member.Repository
	workspaces workspace.Repository
	linkTokens LinkTokenService
	clock      Clock
	caller     caller.Resolver
	configs    ConfigValidator
	events     EventPublisher
	logger     *slog.Logger
}

type Caller = caller.Caller

type Config struct {
	Projects   project.Repository
	Members    member.Repository
	Workspaces workspace.Repository
	LinkTokens LinkTokenService
	Clock      Clock
	Caller     caller.Resolver
	Configs    ConfigValidator
	Events     EventPublisher
	Logger     *slog.Logger
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Projects == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if cfg.Members == nil {
		return nil, fmt.Errorf("project member repository is required")
	}
	if cfg.Workspaces == nil {
		return nil, fmt.Errorf("project workspace repository is required")
	}
	if cfg.LinkTokens == nil {
		return nil, fmt.Errorf("project link token issuer is required")
	}
	if cfg.Caller == nil {
		return nil, fmt.Errorf("project caller resolver is required")
	}
	if cfg.Configs == nil {
		return nil, fmt.Errorf("project member config validator is required")
	}
	if cfg.Events == nil {
		return nil, fmt.Errorf("project event publisher is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("service", "project")
	}
	return &Service{
		projects:   cfg.Projects,
		members:    cfg.Members,
		workspaces: cfg.Workspaces,
		linkTokens: cfg.LinkTokens,
		clock:      clock,
		caller:     cfg.Caller,
		configs:    cfg.Configs,
		events:     cfg.Events,
		logger:     logger,
	}, nil
}

type SaveProjectInput struct {
	ID         types.ProjectID
	LocationID types.LocationID
	Root       string
	Title      string
	Status     project.Status
}

type CreateProjectInput struct {
	LocationID types.LocationID
	Root       string
	Title      string
	Status     project.Status
}

func (s *Service) CreateProject(ctx context.Context, input CreateProjectInput) (project.Project, error) {
	root := strings.TrimSpace(input.Root)
	if root == "" {
		return project.Project{}, fmt.Errorf("project root is required")
	}
	projectID := ProjectIDForLocationRoot(input.LocationID, root)
	if projectID == "" {
		return project.Project{}, fmt.Errorf("project id could not be derived")
	}
	return s.SaveProject(ctx, SaveProjectInput{
		ID:         projectID,
		LocationID: input.LocationID,
		Root:       root,
		Title:      strings.TrimSpace(input.Title),
		Status:     input.Status,
	})
}

func (s *Service) SaveProject(ctx context.Context, input SaveProjectInput) (project.Project, error) {
	if s == nil {
		return project.Project{}, fmt.Errorf("project service is nil")
	}
	now := s.now()
	// A project is owned by a user. Resolve the caller and require a UserID so we
	// never persist an owner-less project: a member-only caller (no UserID) is
	// rejected loudly rather than silently writing UserID="". This is what gates
	// later ownership checks (requireOwnedProject) and link-token minting.
	c, err := s.resolveCaller(ctx)
	if err != nil {
		return project.Project{}, err
	}
	userID := c.UserID
	if userID == "" {
		return project.Project{}, fmt.Errorf("save project requires an owning user")
	}
	agg, err := project.New(project.NewInput{
		ID:         input.ID,
		LocationID: input.LocationID,
		Root:       input.Root,
		Title:      input.Title,
		UserID:     userID,
		Status:     input.Status,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		return project.Project{}, err
	}
	saved, err := s.projects.Save(ctx, agg.Record())
	if err != nil {
		return project.Project{}, err
	}
	return project.Wrap(saved)
}

// UpdateProjectInput is the user-facing edit path: rename and/or recolor a
// project the caller owns. Title/Customization are pointers so a nil leaves the
// stored value untouched. ProjectID identifies the target; identity, root,
// status, owner, and createdAt are never altered here.
type UpdateProjectInput struct {
	ProjectID     types.ProjectID
	Title         *string
	Customization *project.Customization
}

// UpdateProject edits the user-editable fields of an owned project. Unlike
// SaveProject (an upsert that re-owns the caller and clobbers createdAt /
// customization), this is a load-modify-save that requires the caller to
// already own the project and preserves everything it does not explicitly
// change. This is the path the UI rename/recolor affordance calls.
func (s *Service) UpdateProject(ctx context.Context, input UpdateProjectInput) (project.Project, error) {
	if s == nil {
		return project.Project{}, fmt.Errorf("project service is nil")
	}
	c, err := s.resolveCaller(ctx)
	if err != nil {
		return project.Project{}, err
	}
	current, err := s.GetProject(ctx, input.ProjectID)
	if err != nil {
		return project.Project{}, err
	}
	if err := requireOwnedProject(c, current); err != nil {
		return project.Project{}, err
	}
	updated := current.Update(project.UpdateInput{
		Title:         input.Title,
		Customization: input.Customization,
	}, s.now())
	saved, err := s.projects.Save(ctx, updated.Record())
	if err != nil {
		return project.Project{}, err
	}
	return project.Wrap(saved)
}

func (s *Service) GetProject(ctx context.Context, projectID types.ProjectID) (project.Project, error) {
	if s == nil {
		return project.Project{}, fmt.Errorf("project service is nil")
	}
	projectID = cleanProjectID(projectID)
	if projectID == "" {
		return project.Project{}, fmt.Errorf("project id is required")
	}
	record, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return project.Project{}, err
	}
	return project.Wrap(record)
}

func (s *Service) ListProjects(ctx context.Context, filter project.Filter) ([]project.Project, error) {
	if s == nil {
		return nil, fmt.Errorf("project service is nil")
	}
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, fmt.Errorf("project limit and offset must be non-negative")
	}
	records, err := s.projects.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]project.Project, 0, len(records))
	for _, record := range records {
		item, err := project.Wrap(record)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// ActiveWorkspaceRoots returns every root that can identify this project for
// directory-based hook attribution. Operational file access must use either
// the stable project root or an explicit member workspace binding; this set is
// only for matching an observed cwd to its owning project.
func (s *Service) ActiveWorkspaceRoots(ctx context.Context, p project.Project) ([]string, error) {
	stored := strings.TrimSpace(p.Root())
	roots := make([]string, 0, 2)
	seen := map[string]struct{}{}
	add := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		if _, exists := seen[root]; exists {
			return
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	add(stored)
	if s == nil || s.workspaces == nil {
		return roots, nil
	}
	records, err := s.workspaces.List(ctx, workspace.Filter{
		ProjectID:      strings.TrimSpace(string(p.ID())),
		LifecycleState: workspace.LifecycleActive,
	})
	if err != nil {
		return nil, fmt.Errorf("list active project workspaces: %w", err)
	}
	projectLocation := normalizeLocationID(string(p.LocationID()))
	projectUser := normalizeWorkspaceUser(p.UserID())
	for _, record := range records {
		if normalizeLocationID(record.LocationID) != projectLocation {
			continue
		}
		if normalizeWorkspaceUser(record.UserID) != projectUser {
			continue
		}
		add(record.Root)
	}
	return roots, nil
}

func normalizeWorkspaceUser(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "local"
	}
	return id
}

// normalizeLocationID folds the empty location and the explicit "local"
// sentinel together, so a project stored without a location still matches its
// local workspaces.
func normalizeLocationID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "local"
	}
	return id
}

func (s *Service) ArchiveProject(ctx context.Context, projectID types.ProjectID) (project.Project, error) {
	if s == nil {
		return project.Project{}, fmt.Errorf("project service is nil")
	}
	current, err := s.GetProject(ctx, projectID)
	if err != nil {
		return project.Project{}, err
	}
	record := current.Record()
	record.Status = project.StatusArchived
	record.UpdatedAt = s.now()
	saved, err := s.projects.Save(ctx, record)
	if err != nil {
		return project.Project{}, err
	}
	return project.Wrap(saved)
}

func (s *Service) DeleteProject(ctx context.Context, projectID types.ProjectID) error {
	if s == nil {
		return fmt.Errorf("project service is nil")
	}
	current, err := s.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if current.Status() != project.StatusArchived {
		return fmt.Errorf("project %q: %w", current.ID(), project.ErrNotArchived)
	}
	// Deletion removes only the project record; the project's files on disk are
	// never touched, so a missing root directory is irrelevant here.
	return s.projects.Delete(ctx, current.ID())
}

func (s *Service) resolveCaller(ctx context.Context) (Caller, error) {
	c, err := s.caller.ResolveCaller(ctx)
	if err != nil {
		return Caller{}, fmt.Errorf("resolve project caller: %w", err)
	}
	c = c.Normalize()
	if c.UserID == "" && c.MemberID == "" {
		return Caller{}, fmt.Errorf("project caller user id or member id is required")
	}
	return c, nil
}

func requireVisibleProject(c Caller, p project.Project) error {
	if c.UserID != "" && strings.TrimSpace(p.UserID()) == c.UserID {
		return nil
	}
	if c.ProjectID != "" && p.ID() == c.ProjectID {
		return nil
	}
	return fmt.Errorf("project %s is not visible to caller", p.ID())
}

func requireOwnedProject(c Caller, p project.Project) error {
	if c.UserID != "" && strings.TrimSpace(p.UserID()) == c.UserID {
		return nil
	}
	return fmt.Errorf("project %s is not owned by caller", p.ID())
}

func (s *Service) publishMemberLifecycle(eventType string, rosterMember member.Record) error {
	if s.events == nil {
		return fmt.Errorf("project event publisher is required")
	}
	return s.events.Publish(eventbus.TopicSpaceMemberLifecycle, eventbus.SpaceMemberLifecycleEvent{
		UserID:    rosterMember.UserID,
		ProjectID: rosterMember.ProjectID,
		// SpaceID carries the coordination-boundary id consumed by the harness
		// daemon. In the project-first model the project is that boundary; the
		// event field is renamed to ProjectID in the M5 eventbus pass.
		SpaceID:        rosterMember.ProjectID,
		MemberID:       string(rosterMember.ID),
		ChannelID:      rosterMember.ChannelID,
		DisplayName:    rosterMember.DisplayName,
		MemberType:     rosterMember.MemberType,
		EventType:      eventType,
		LifecycleState: rosterMember.LifecycleState,
		HarnessKind:    rosterMember.HarnessKind,
		Model:          rosterMember.Model,
		Effort:         rosterMember.Effort,
		PermissionMode: rosterMember.PermissionMode,
		ConfigRef:      rosterMember.ConfigRef,
		Timestamp:      s.clock.Now().UTC(),
	})
}

func (s *Service) now() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock.Now().UTC()
}

func cleanProjectID(id types.ProjectID) types.ProjectID {
	return types.ProjectID(strings.TrimSpace(string(id)))
}

func ProjectIDForRoot(root string) types.ProjectID {
	return ProjectIDForLocationRoot("local", root)
}

func ProjectIDForLocationRoot(locationID types.LocationID, root string) types.ProjectID {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	locationID = types.LocationID(strings.TrimSpace(string(locationID)))
	if locationID == "" {
		locationID = "local"
	}
	cleaned := filepath.Clean(root)
	base := filepath.Base(cleaned)
	slug := slugifyProjectID(base)
	if slug == "" {
		slug = "project"
	}
	// #nosec G401 -- durable compatibility identifier; not used for cryptographic security.
	sum := sha1.Sum([]byte(string(locationID) + "\x00" + cleaned))
	return types.ProjectID(fmt.Sprintf("%s-%s", slug, hex.EncodeToString(sum[:])[:8]))
}

func slugifyProjectID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
