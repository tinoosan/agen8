package app

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestService_SaveAndGetProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projects := newProjectRepoSpy()
	svc := newServiceForTest(t, projects, newClusterRepoSpy(), newSpaceLoaderSpy(nil))

	saved, err := svc.SaveProject(ctx, SaveProjectInput{
		ID:         "project-1",
		LocationID: "local",
		Root:       "/tmp/project",
		Title:      "Project",
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if saved.ID() != "project-1" || saved.LocationID() != "local" || saved.Status() != project.StatusOpen {
		t.Fatalf("saved project = %+v", saved.Record())
	}
	loaded, err := svc.GetProject(ctx, "project-1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if loaded.Title() != "Project" {
		t.Fatalf("loaded project = %+v", loaded.Record())
	}
	if projects.saved[0].CreatedAt.IsZero() || !projects.saved[0].CreatedAt.Equal(fixedNow) {
		t.Fatalf("saved timestamp = %+v", projects.saved[0])
	}
}

func TestService_CreateProjectDerivesStableID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projects := newProjectRepoSpy()
	svc := newServiceForTest(t, projects, newClusterRepoSpy(), newSpaceLoaderSpy(nil))

	created, err := svc.CreateProject(ctx, CreateProjectInput{
		LocationID: "ssh-prod",
		Root:       "/tmp/Launch Kit",
		Title:      "Launch",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if created.ID() == "" || created.ID() != ProjectIDForLocationRoot("ssh-prod", "/tmp/Launch Kit") {
		t.Fatalf("created id = %q", created.ID())
	}
	if created.LocationID() != "ssh-prod" {
		t.Fatalf("created location = %q", created.LocationID())
	}
	if created.Title() != "Launch" || created.Status() != project.StatusOpen {
		t.Fatalf("created project = %+v", created.Record())
	}
	loaded, err := svc.GetProject(ctx, created.ID())
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if loaded.ID() != created.ID() {
		t.Fatalf("loaded id = %q want %q", loaded.ID(), created.ID())
	}
}

func TestService_ArchiveThenDeleteProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projects := newProjectRepoSpy()
	svc := newServiceForTest(t, projects, newClusterRepoSpy(), newSpaceLoaderSpy(nil))
	created, err := svc.SaveProject(ctx, SaveProjectInput{
		ID:    "project-1",
		Root:  "/tmp/project-1",
		Title: "Project",
	})
	if err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := svc.DeleteProject(ctx, created.ID()); err == nil {
		t.Fatalf("expected delete before archive to fail")
	}
	archived, err := svc.ArchiveProject(ctx, created.ID())
	if err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	if archived.Status() != project.StatusArchived {
		t.Fatalf("archived status = %q", archived.Status())
	}
	if err := svc.DeleteProject(ctx, created.ID()); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := svc.GetProject(ctx, created.ID()); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Get deleted error = %v", err)
	}
}

func TestService_ListProjectSpacesOverlaysPinnedTopology(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusters := newClusterRepoSpy()
	mustSaveCluster(t, clusters, cluster.Record{ID: "cluster-1", ProjectID: "project-1", Name: "Launch", Status: cluster.StatusOpen})
	mustSaveRef(t, clusters, cluster.SpaceRefRecord{ClusterID: "cluster-1", SpaceID: "space-2", SortOrder: 2, Pinned: false})
	mustSaveRef(t, clusters, cluster.SpaceRefRecord{ClusterID: "cluster-1", SpaceID: "space-1", SortOrder: 1, Pinned: true})
	spaces := newSpaceLoaderSpy([]spacedomain.SpaceRecord{
		{ID: "space-2", ProjectID: "project-1", Title: "Beta", Status: spacedomain.SpaceStatusOpen},
		{ID: "space-1", ProjectID: "project-1", Title: "Alpha", Status: spacedomain.SpaceStatusOpen},
		{ID: "space-3", ProjectID: "project-1", Title: "Gamma", Status: spacedomain.SpaceStatusClosed},
	})
	spaces.members = []member.Record{
		{ID: "member-fred", ProjectID: "project-1", SpaceID: "space-1", DisplayName: "Fred", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive},
		{ID: "member-removed", ProjectID: "project-1", SpaceID: "space-1", DisplayName: "Removed", MemberType: member.TypeWorker, LifecycleState: member.LifecycleRemoved},
	}
	svc := newServiceForTest(t, newProjectRepoSpy(), clusters, spaces)

	listed, err := svc.ListProjectSpaces(ctx, "project-1")
	if err != nil {
		t.Fatalf("ListProjectSpaces: %v", err)
	}
	got := []spacedomain.SpaceID{listed[0].SpaceID, listed[1].SpaceID, listed[2].SpaceID}
	want := []spacedomain.SpaceID{"space-1", "space-2", "space-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("space order = %v want %v; listed=%+v", got, want, listed)
	}
	if !listed[0].Pinned || listed[0].SortOrder != 1 || !listed[0].SpaceOpen || listed[2].SpaceOpen {
		t.Fatalf("overlay fields = %+v", listed)
	}
	if spaces.listFilters[0].ProjectID != "project-1" {
		t.Fatalf("space list filters = %+v", spaces.listFilters)
	}
	if len(spaces.memberFilters) != 1 || spaces.memberFilters[0].ProjectID != "project-1" || spaces.memberFilters[0].LifecycleState != member.LifecycleActive {
		t.Fatalf("member list filters = %+v", spaces.memberFilters)
	}
	if len(listed[0].Members) != 1 || listed[0].Members[0].MemberID != "member-fred" || listed[0].Members[0].Label != "Fred" {
		t.Fatalf("member labels = %+v", listed[0].Members)
	}
}

func TestService_SaveClusterSpaceRequiresSameProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusters := newClusterRepoSpy()
	mustSaveCluster(t, clusters, cluster.Record{ID: "cluster-1", ProjectID: "project-1", Name: "Launch", Status: cluster.StatusOpen})
	spaces := newSpaceLoaderSpy([]spacedomain.SpaceRecord{
		{ID: "space-1", ProjectID: "project-2", Title: "Wrong", Status: spacedomain.SpaceStatusOpen},
	})
	svc := newServiceForTest(t, newProjectRepoSpy(), clusters, spaces)

	if _, err := svc.SaveClusterSpace(ctx, SaveClusterSpaceInput{
		ProjectID: "project-1",
		ClusterID: "cluster-1",
		SpaceID:   "space-1",
	}); err == nil {
		t.Fatalf("expected cross-project space ref to fail")
	}
	if len(clusters.savedRefs) != 0 {
		t.Fatalf("saved refs = %+v", clusters.savedRefs)
	}
}

func TestService_SaveClusterSpaceAndRemove(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusters := newClusterRepoSpy()
	mustSaveCluster(t, clusters, cluster.Record{ID: "cluster-1", ProjectID: "project-1", Name: "Launch", Status: cluster.StatusOpen})
	spaces := newSpaceLoaderSpy([]spacedomain.SpaceRecord{
		{ID: "space-1", ProjectID: "project-1", Title: "Alpha", Status: spacedomain.SpaceStatusOpen},
	})
	svc := newServiceForTest(t, newProjectRepoSpy(), clusters, spaces)

	ref, err := svc.SaveClusterSpace(ctx, SaveClusterSpaceInput{
		ProjectID: "project-1",
		ClusterID: "cluster-1",
		SpaceID:   "space-1",
		SortOrder: 7,
		Pinned:    true,
	})
	if err != nil {
		t.Fatalf("SaveClusterSpace: %v", err)
	}
	if ref.SpaceID != "space-1" || !ref.Pinned || ref.SortOrder != 7 {
		t.Fatalf("saved ref = %+v", ref)
	}
	if err := svc.RemoveClusterSpace(ctx, "project-1", "cluster-1", "space-1"); err != nil {
		t.Fatalf("RemoveClusterSpace: %v", err)
	}
	if _, ok := clusters.refs["cluster-1"]["space-1"]; ok {
		t.Fatalf("ref was not removed")
	}
}

func TestService_ListClustersIncludesRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clusters := newClusterRepoSpy()
	mustSaveCluster(t, clusters, cluster.Record{ID: "cluster-1", ProjectID: "project-1", Name: "Launch", Status: cluster.StatusOpen})
	mustSaveRef(t, clusters, cluster.SpaceRefRecord{ClusterID: "cluster-1", SpaceID: "space-1", SortOrder: 1, Pinned: true})
	svc := newServiceForTest(t, newProjectRepoSpy(), clusters, newSpaceLoaderSpy(nil))

	listed, err := svc.ListClusters(ctx, "project-1")
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "cluster-1" || len(listed[0].Spaces) != 1 {
		t.Fatalf("listed clusters = %+v", listed)
	}
}

func newServiceForTest(t *testing.T, projects *projectRepoSpy, clusters *clusterRepoSpy, spaces *spaceLoaderSpy) *Service {
	t.Helper()
	svc, err := NewService(Config{
		Projects: projects,
		Clusters: clusters,
		Spaces:   spaces,
		Clock:    fixedClock{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

var fixedNow = time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return fixedNow }

type projectRepoSpy struct {
	records map[types.ProjectID]project.Record
	saved   []project.Record
}

func newProjectRepoSpy() *projectRepoSpy {
	return &projectRepoSpy{records: map[types.ProjectID]project.Record{}}
}

func (r *projectRepoSpy) Get(_ context.Context, id types.ProjectID) (project.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return project.Record{}, project.ErrNotFound
	}
	return record, nil
}

func (r *projectRepoSpy) List(_ context.Context, filter project.Filter) ([]project.Record, error) {
	var out []project.Record
	for _, record := range r.records {
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *projectRepoSpy) Save(_ context.Context, record project.Record) (project.Record, error) {
	r.saved = append(r.saved, record)
	r.records[record.ID] = record
	return record, nil
}

func (r *projectRepoSpy) Delete(_ context.Context, id types.ProjectID) error {
	if _, ok := r.records[id]; !ok {
		return project.ErrNotFound
	}
	delete(r.records, id)
	return nil
}

type clusterRepoSpy struct {
	records   map[cluster.ID]cluster.Record
	refs      map[cluster.ID]map[spacedomain.SpaceID]cluster.SpaceRefRecord
	savedRefs []cluster.SpaceRefRecord
}

func newClusterRepoSpy() *clusterRepoSpy {
	return &clusterRepoSpy{
		records: map[cluster.ID]cluster.Record{},
		refs:    map[cluster.ID]map[spacedomain.SpaceID]cluster.SpaceRefRecord{},
	}
}

func (r *clusterRepoSpy) List(_ context.Context, filter cluster.Filter) ([]cluster.Record, error) {
	var out []cluster.Record
	for _, record := range r.records {
		if filter.ProjectID != "" && record.ProjectID != filter.ProjectID {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *clusterRepoSpy) ListSpaces(_ context.Context, clusterID cluster.ID) ([]cluster.SpaceRefRecord, error) {
	var out []cluster.SpaceRefRecord
	for _, ref := range r.refs[clusterID] {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].SpaceID < out[j].SpaceID
	})
	return out, nil
}

func (r *clusterRepoSpy) Save(_ context.Context, record cluster.Record) (cluster.Record, error) {
	r.records[record.ID] = record
	return record, nil
}

func (r *clusterRepoSpy) SaveSpace(_ context.Context, ref cluster.SpaceRefRecord) (cluster.SpaceRefRecord, error) {
	if _, ok := r.records[ref.ClusterID]; !ok {
		return cluster.SpaceRefRecord{}, cluster.ErrNotFound
	}
	if r.refs[ref.ClusterID] == nil {
		r.refs[ref.ClusterID] = map[spacedomain.SpaceID]cluster.SpaceRefRecord{}
	}
	r.savedRefs = append(r.savedRefs, ref)
	r.refs[ref.ClusterID][ref.SpaceID] = ref
	return ref, nil
}

func (r *clusterRepoSpy) RemoveSpace(_ context.Context, clusterID cluster.ID, spaceID spacedomain.SpaceID) error {
	if r.refs[clusterID] == nil {
		return cluster.ErrNotFound
	}
	if _, ok := r.refs[clusterID][spaceID]; !ok {
		return cluster.ErrNotFound
	}
	delete(r.refs[clusterID], spaceID)
	return nil
}

func mustSaveCluster(t *testing.T, repo *clusterRepoSpy, record cluster.Record) {
	t.Helper()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = fixedNow
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	if _, err := repo.Save(context.Background(), record); err != nil {
		t.Fatalf("save cluster fixture: %v", err)
	}
}

func mustSaveRef(t *testing.T, repo *clusterRepoSpy, ref cluster.SpaceRefRecord) {
	t.Helper()
	if _, err := repo.SaveSpace(context.Background(), ref); err != nil {
		t.Fatalf("save ref fixture: %v", err)
	}
}

type spaceLoaderSpy struct {
	spaces      map[spacedomain.SpaceID]spacedomain.SpaceRecord
	members     []member.Record
	listFilters []spacedomain.SpaceFilter
	memberFilters []member.Filter
}

func newSpaceLoaderSpy(spaces []spacedomain.SpaceRecord) *spaceLoaderSpy {
	out := &spaceLoaderSpy{spaces: map[spacedomain.SpaceID]spacedomain.SpaceRecord{}}
	for _, space := range spaces {
		out.spaces[space.ID] = space
	}
	return out
}

func (s *spaceLoaderSpy) Get(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := s.spaces[id]
	if !ok {
		return spacedomain.SpaceRecord{}, errors.New("space not found")
	}
	return space, nil
}

func (s *spaceLoaderSpy) List(_ context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
	s.listFilters = append(s.listFilters, filter)
	var out []spacedomain.SpaceRecord
	for _, space := range s.spaces {
		if filter.ProjectID != "" && string(space.ProjectID) != filter.ProjectID {
			continue
		}
		out = append(out, space)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *spaceLoaderSpy) ListMembers(_ context.Context, filter member.Filter) ([]member.Record, error) {
	s.memberFilters = append(s.memberFilters, filter)
	var out []member.Record
	for _, rosterMember := range s.members {
		if filter.ProjectID != "" && rosterMember.ProjectID != filter.ProjectID {
			continue
		}
		if filter.SpaceID != "" && rosterMember.SpaceID != filter.SpaceID {
			continue
		}
		if filter.LifecycleState != "" && rosterMember.LifecycleState != filter.LifecycleState {
			continue
		}
		out = append(out, rosterMember)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
