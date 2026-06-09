package app

import (
	"context"
	"crypto/sha1"
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
	linkTokens LinkTokenIssuer
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
	LinkTokens LinkTokenIssuer
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
		return fmt.Errorf("project %q must be archived before deletion", current.ID())
	}
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
