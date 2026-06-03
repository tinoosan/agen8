package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	filedomain "github.com/tinoosan/agen8-mcp-server/internal/services/file/domain/file"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestServiceListDirUsesFileRepositoryAfterProjectRootValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projectRoot := t.TempDir()
	repo := &spyFileRepository{
		stats: map[filedomain.Reference]filedomain.Info{
			{LocationID: "local", Path: projectRoot}: {IsDir: true, ModifiedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)},
		},
		dirs: map[filedomain.Reference][]filedomain.Entry{
			{LocationID: "local", Path: projectRoot}: {
				{Name: "notes.md", Info: filedomain.Info{Size: 12, ModifiedAt: time.Date(2026, 5, 31, 10, 1, 0, 0, time.UTC)}},
			},
		},
	}
	svc := newTestService(t, projectRoot, repo)

	result, err := svc.ListDir(ctx, ListDirInput{ProjectRoot: projectRoot, Path: "/project"})
	require.NoError(t, err)
	require.Equal(t, []filedomain.Reference{{LocationID: "local", Path: projectRoot}}, repo.statPaths)
	require.Equal(t, []filedomain.Reference{{LocationID: "local", Path: projectRoot}}, repo.listDirPaths)
	require.Len(t, result.Entries, 1)
	require.Equal(t, "/project/notes.md", result.Entries[0].Path)
	require.Equal(t, int64(12), result.Entries[0].Size)
}

func TestServiceGetRejectsUnregisteredProjectRootBeforeRepositoryRead(t *testing.T) {
	t.Parallel()
	repo := &spyFileRepository{}
	svc := newTestService(t, t.TempDir(), repo)

	_, err := svc.Get(context.Background(), GetInput{
		ProjectRoot: t.TempDir(),
		Path:        "/project/notes.md",
	})
	require.ErrorContains(t, err, "projectRoot is not registered")
	require.Empty(t, repo.statPaths)
	require.Empty(t, repo.readPaths)
}

func TestServiceListDirUsesProjectLocationForRemoteProjectRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := &spyFileRepository{
		stats: map[filedomain.Reference]filedomain.Info{
			{LocationID: "ssh-build", Path: "/srv/app"}: {IsDir: true, ModifiedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)},
		},
		dirs: map[filedomain.Reference][]filedomain.Entry{
			{LocationID: "ssh-build", Path: "/srv/app"}: {
				{Name: "README.md", Info: filedomain.Info{Size: 16, ModifiedAt: time.Date(2026, 5, 31, 10, 1, 0, 0, time.UTC)}},
			},
		},
	}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})

	result, err := svc.ListDir(ctx, ListDirInput{ProjectRoot: "/srv/app", Path: "/project"})
	require.NoError(t, err)
	require.Equal(t, []filedomain.Reference{{LocationID: "ssh-build", Path: "/srv/app"}}, repo.statPaths)
	require.Equal(t, []filedomain.Reference{{LocationID: "ssh-build", Path: "/srv/app"}}, repo.listDirPaths)
	require.Equal(t, "/project/README.md", result.Entries[0].Path)
}

func TestServiceGetUsesProjectIDBeforeProjectRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := &spyFileRepository{
		stats: map[filedomain.Reference]filedomain.Info{
			{LocationID: "ssh-build", Path: "/srv/app/README.md"}: {Size: 7, ModifiedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)},
		},
	}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})

	_, err := svc.Get(ctx, GetInput{
		ProjectID:   "project-test-0",
		ProjectRoot: "/same/path/on/local/host",
		Path:        "/project/README.md",
		MaxBytes:    1024,
	})
	require.NoError(t, err)
	require.Equal(t, []filedomain.Reference{{LocationID: "ssh-build", Path: "/srv/app/README.md"}}, repo.statPaths)
	require.Equal(t, []filedomain.Reference{{LocationID: "ssh-build", Path: "/srv/app/README.md"}}, repo.readPaths)
}

func newTestService(t *testing.T, projectRoot string, repo filedomain.Repository) *Service {
	t.Helper()
	return newTestServiceWithProjects(t, repo, testProject{root: projectRoot, locationID: "local"})
}

func newTestServiceWithProjects(t *testing.T, repo filedomain.Repository, projects ...testProject) *Service {
	t.Helper()
	svc, err := NewService(Config{
		Files: repo,
		Projects: staticProjectLoader{
			projects: projects,
		},
	})
	require.NoError(t, err)
	return svc
}

type testProject struct {
	root       string
	locationID types.LocationID
}

type staticProjectLoader struct {
	projects []testProject
}

func (l staticProjectLoader) GetProject(_ context.Context, projectID types.ProjectID) (projectdomain.Project, error) {
	projects, err := l.ListProjects(context.Background(), projectdomain.Filter{})
	if err != nil {
		return projectdomain.Project{}, err
	}
	for _, project := range projects {
		if project.ID() == projectID {
			return project, nil
		}
	}
	return projectdomain.Project{}, fmt.Errorf("project not found")
}

func (l staticProjectLoader) ListProjects(context.Context, projectdomain.Filter) ([]projectdomain.Project, error) {
	out := make([]projectdomain.Project, 0, len(l.projects))
	for i, input := range l.projects {
		project, err := projectdomain.New(projectdomain.NewInput{
			ID:         projectdomainID(i),
			LocationID: input.locationID,
			Root:       input.root,
			CreatedAt:  time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, project)
	}
	return out, nil
}

func projectdomainID(index int) types.ProjectID {
	return types.ProjectID(fmt.Sprintf("project-test-%d", index))
}

type spyFileRepository struct {
	stats map[filedomain.Reference]filedomain.Info
	dirs  map[filedomain.Reference][]filedomain.Entry

	statPaths    []filedomain.Reference
	listDirPaths []filedomain.Reference
	readPaths    []filedomain.Reference
}

func (r *spyFileRepository) Stat(_ context.Context, ref filedomain.Reference) (filedomain.Info, error) {
	r.statPaths = append(r.statPaths, ref)
	if r.stats == nil {
		return filedomain.Info{}, nil
	}
	return r.stats[ref], nil
}

func (r *spyFileRepository) ListDir(_ context.Context, ref filedomain.Reference) ([]filedomain.Entry, error) {
	r.listDirPaths = append(r.listDirPaths, ref)
	return append([]filedomain.Entry(nil), r.dirs[ref]...), nil
}

func (r *spyFileRepository) Read(_ context.Context, ref filedomain.Reference, _ int64) (filedomain.Content, error) {
	r.readPaths = append(r.readPaths, ref)
	return filedomain.Content{}, nil
}

func (*spyFileRepository) CreateDir(context.Context, filedomain.Reference) error  { return nil }
func (*spyFileRepository) CreateFile(context.Context, filedomain.Reference) error { return nil }
func (*spyFileRepository) Move(context.Context, filedomain.Reference, filedomain.Reference) error {
	return nil
}
func (*spyFileRepository) Copy(context.Context, filedomain.Reference, filedomain.Reference) error {
	return nil
}
func (*spyFileRepository) Delete(context.Context, filedomain.Reference) error { return nil }
func (*spyFileRepository) WriteFile(context.Context, filedomain.Reference, []byte) error {
	return nil
}
