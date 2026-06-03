package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Service struct {
	projects project.Repository
	clusters cluster.Repository
	spaces   SpaceLoader
	clock    Clock
}

type Config struct {
	Projects project.Repository
	Clusters cluster.Repository
	Spaces   SpaceLoader
	Clock    Clock
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Projects == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if cfg.Clusters == nil {
		return nil, fmt.Errorf("project cluster repository is required")
	}
	if cfg.Spaces == nil {
		return nil, fmt.Errorf("project space loader is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &Service{
		projects: cfg.Projects,
		clusters: cfg.Clusters,
		spaces:   cfg.Spaces,
		clock:    clock,
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
	agg, err := project.New(project.NewInput{
		ID:         input.ID,
		LocationID: input.LocationID,
		Root:       input.Root,
		Title:      input.Title,
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

type ProjectSpaceView struct {
	ProjectID types.ProjectID
	SpaceID   spacedomain.SpaceID
	Status    string
	SortOrder int
	Pinned    bool
	Title     string
	SpaceOpen bool
	Members   []SpaceMemberView
}

type SpaceMemberView struct {
	MemberID member.ID
	Label    string
}

func (s *Service) ListProjectSpaces(ctx context.Context, projectID types.ProjectID) ([]ProjectSpaceView, error) {
	if s == nil {
		return nil, fmt.Errorf("project service is nil")
	}
	projectID = cleanProjectID(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	spaces, err := s.spaces.List(ctx, spacedomain.SpaceFilter{ProjectID: string(projectID)})
	if err != nil {
		return nil, err
	}
	refs, err := s.clusterRefsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	members, err := s.spaces.ListMembers(ctx, member.Filter{
		ProjectID:      string(projectID),
		LifecycleState: member.LifecycleActive,
		Limit:          1000,
	})
	if err != nil {
		return nil, fmt.Errorf("list project space members: %w", err)
	}
	membersBySpace := make(map[spacedomain.SpaceID][]SpaceMemberView)
	for _, rosterMember := range members {
		label := strings.TrimSpace(rosterMember.DisplayName)
		if label == "" {
			label = strings.TrimSpace(rosterMember.MemberType)
		}
		if label == "" {
			label = string(rosterMember.ID)
		}
		membersBySpace[spacedomain.SpaceID(rosterMember.SpaceID)] = append(membersBySpace[spacedomain.SpaceID(rosterMember.SpaceID)], SpaceMemberView{
			MemberID: rosterMember.ID,
			Label:    label,
		})
	}
	out := make([]ProjectSpaceView, 0, len(spaces))
	for _, space := range spaces {
		ref := refs[space.ID]
		out = append(out, ProjectSpaceView{
			ProjectID: projectID,
			SpaceID:   space.ID,
			Status:    strings.TrimSpace(space.Status),
			SortOrder: ref.SortOrder,
			Pinned:    ref.Pinned,
			Title:     strings.TrimSpace(space.Title),
			SpaceOpen: strings.TrimSpace(space.Status) == spacedomain.SpaceStatusOpen,
			Members:   membersBySpace[space.ID],
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].SpaceOpen != out[j].SpaceOpen {
			return out[i].SpaceOpen
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

type ClusterView struct {
	ID        cluster.ID
	ProjectID types.ProjectID
	Name      string
	Status    cluster.Status
	Spaces    []cluster.SpaceRefRecord
}

type SaveClusterInput struct {
	ID        cluster.ID
	ProjectID types.ProjectID
	Name      string
	Status    cluster.Status
}

func (s *Service) SaveCluster(ctx context.Context, input SaveClusterInput) (ClusterView, error) {
	if s == nil {
		return ClusterView{}, fmt.Errorf("project service is nil")
	}
	now := s.now()
	agg, err := cluster.New(cluster.NewInput{
		ID:        input.ID,
		ProjectID: input.ProjectID,
		Name:      input.Name,
		Status:    input.Status,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return ClusterView{}, err
	}
	saved, err := s.clusters.Save(ctx, agg.Record())
	if err != nil {
		return ClusterView{}, err
	}
	wrapped, err := cluster.Wrap(saved)
	if err != nil {
		return ClusterView{}, err
	}
	return ClusterView{
		ID:        wrapped.ID(),
		ProjectID: wrapped.ProjectID(),
		Name:      wrapped.Name(),
		Status:    wrapped.Status(),
	}, nil
}

func (s *Service) ListClusters(ctx context.Context, projectID types.ProjectID) ([]ClusterView, error) {
	if s == nil {
		return nil, fmt.Errorf("project service is nil")
	}
	projectID = cleanProjectID(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	records, err := s.clusters.List(ctx, cluster.Filter{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	out := make([]ClusterView, 0, len(records))
	for _, record := range records {
		agg, err := cluster.Wrap(record)
		if err != nil {
			return nil, err
		}
		refs, err := s.clusters.ListSpaces(ctx, agg.ID())
		if err != nil {
			return nil, err
		}
		out = append(out, ClusterView{
			ID:        agg.ID(),
			ProjectID: agg.ProjectID(),
			Name:      agg.Name(),
			Status:    agg.Status(),
			Spaces:    refs,
		})
	}
	return out, nil
}

type SaveClusterSpaceInput struct {
	ClusterID cluster.ID
	ProjectID types.ProjectID
	SpaceID   spacedomain.SpaceID
	SortOrder int
	Pinned    bool
}

func (s *Service) SaveClusterSpace(ctx context.Context, input SaveClusterSpaceInput) (cluster.SpaceRefRecord, error) {
	if s == nil {
		return cluster.SpaceRefRecord{}, fmt.Errorf("project service is nil")
	}
	projectID := cleanProjectID(input.ProjectID)
	if projectID == "" {
		return cluster.SpaceRefRecord{}, fmt.Errorf("project id is required")
	}
	ref, err := cluster.NewSpaceRef(cluster.NewSpaceRefInput{
		ClusterID: input.ClusterID,
		SpaceID:   input.SpaceID,
		SortOrder: input.SortOrder,
		Pinned:    input.Pinned,
	})
	if err != nil {
		return cluster.SpaceRefRecord{}, err
	}
	if err := s.requireClusterInProject(ctx, ref.ClusterID(), projectID); err != nil {
		return cluster.SpaceRefRecord{}, err
	}
	if err := s.requireSpaceInProject(ctx, ref.SpaceID(), projectID); err != nil {
		return cluster.SpaceRefRecord{}, err
	}
	return s.clusters.SaveSpace(ctx, ref.Record())
}

func (s *Service) RemoveClusterSpace(ctx context.Context, projectID types.ProjectID, clusterID cluster.ID, spaceID spacedomain.SpaceID) error {
	if s == nil {
		return fmt.Errorf("project service is nil")
	}
	projectID = cleanProjectID(projectID)
	if projectID == "" {
		return fmt.Errorf("project id is required")
	}
	ref, err := cluster.NewSpaceRef(cluster.NewSpaceRefInput{ClusterID: clusterID, SpaceID: spaceID})
	if err != nil {
		return err
	}
	if err := s.requireClusterInProject(ctx, ref.ClusterID(), projectID); err != nil {
		return err
	}
	return s.clusters.RemoveSpace(ctx, ref.ClusterID(), ref.SpaceID())
}

func (s *Service) clusterRefsByProject(ctx context.Context, projectID types.ProjectID) (map[spacedomain.SpaceID]cluster.SpaceRefRecord, error) {
	clusters, err := s.clusters.List(ctx, cluster.Filter{ProjectID: projectID, Status: cluster.StatusOpen})
	if err != nil {
		return nil, err
	}
	out := map[spacedomain.SpaceID]cluster.SpaceRefRecord{}
	for _, item := range clusters {
		refs, err := s.clusters.ListSpaces(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if _, exists := out[ref.SpaceID]; !exists || ref.Pinned {
				out[ref.SpaceID] = ref
			}
		}
	}
	return out, nil
}

func (s *Service) requireClusterInProject(ctx context.Context, clusterID cluster.ID, projectID types.ProjectID) error {
	clusters, err := s.clusters.List(ctx, cluster.Filter{ProjectID: projectID})
	if err != nil {
		return err
	}
	for _, item := range clusters {
		if item.ID == clusterID {
			return nil
		}
	}
	return fmt.Errorf("cluster %q is not in project %q", clusterID, projectID)
}

func (s *Service) requireSpaceInProject(ctx context.Context, spaceID spacedomain.SpaceID, projectID types.ProjectID) error {
	space, err := s.spaces.Get(ctx, spaceID)
	if err != nil {
		return err
	}
	if types.ProjectID(strings.TrimSpace(string(space.ProjectID))) != projectID {
		return fmt.Errorf("space %q is not in project %q", spaceID, projectID)
	}
	return nil
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
