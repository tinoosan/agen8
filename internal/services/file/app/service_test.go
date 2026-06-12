package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8/internal/core/types"
	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
	fileinfra "github.com/tinoosan/agen8/internal/services/file/infra"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
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

// ResolveRoot mirrors the production resolver's fallback path: with no
// workspace records to consult, the effective root is the stored project root.
// The workspace-sourced override is exercised in the project app tests where
// the real workspace repository lives.
func (l staticProjectLoader) ResolveRoot(_ context.Context, p projectdomain.Project) string {
	return p.Root()
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

// TestUploadGetRoundTripsPNGAttachment pins the attachment byte path end to
// end through the real local repository: binary bytes uploaded via base64
// come back from Get byte-identical and classified as an image.
func TestUploadGetRoundTripsPNGAttachment(t *testing.T) {
	projectRoot := t.TempDir()
	svc := newTestService(t, projectRoot, fileinfra.NewLocalRepository())

	// A real, valid 1x1 transparent PNG (signature + IHDR + IDAT + IEND).
	pngBytes, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(pngBytes, []byte("\x89PNG\r\n\x1a\n")), "fixture must be a real PNG")

	const vpath = "/project/.agen8/attachments/task-1/build-screenshot.png"
	uploaded, err := svc.Upload(context.Background(), UploadInput{
		ProjectID: "project-test-0",
		Path:      vpath,
		BytesB64:  base64.StdEncoding.EncodeToString(pngBytes),
	})
	require.NoError(t, err)
	require.Equal(t, vpath, uploaded.Path)

	got, err := svc.Get(context.Background(), GetInput{
		ProjectID: "project-test-0",
		Path:      vpath,
	})
	require.NoError(t, err)
	require.Equal(t, "image", got.ContentKind)
	require.False(t, got.Truncated)
	roundTripped, err := base64.StdEncoding.DecodeString(got.BytesB64)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(pngBytes), sha256.Sum256(roundTripped), "bytes must round-trip identically")
}

// TestBaselineReturnsCommittedContent exercises the real git path: a repo
// with a committed file that has uncommitted working-tree changes must yield
// the HEAD version, while untracked files and non-repos degrade to
// tracked=false rather than erroring.
func TestBaselineReturnsCommittedContent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	projectRoot := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", projectRoot, "-c", "user.email=test@test", "-c", "user.name=test"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	committed := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(projectRoot+"/main.go", []byte(committed), 0o644))
	run("add", "main.go")
	run("commit", "-q", "-m", "initial")
	// Uncommitted working-tree change — the thing the diff view reviews.
	require.NoError(t, os.WriteFile(projectRoot+"/main.go", []byte(committed+"\nfunc added() {}\n"), 0o644))
	// An untracked file has no baseline.
	require.NoError(t, os.WriteFile(projectRoot+"/untracked.txt", []byte("new"), 0o644))

	svc := newTestService(t, projectRoot, fileinfra.NewLocalRepository())

	tracked, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/main.go"})
	require.NoError(t, err)
	require.True(t, tracked.Tracked)
	require.False(t, tracked.Binary)
	require.Equal(t, committed, tracked.Content, "baseline must be the HEAD version, not the working tree")

	untracked, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/untracked.txt"})
	require.NoError(t, err)
	require.False(t, untracked.Tracked)
	require.Empty(t, untracked.Content)
}

func TestBaselineOutsideGitRepoIsUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(projectRoot+"/loose.txt", []byte("no repo here"), 0o644))
	svc := newTestService(t, projectRoot, fileinfra.NewLocalRepository())
	result, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/loose.txt"})
	require.NoError(t, err)
	require.False(t, result.Tracked)
}

func TestBaselineRejectsSymlinkEscapedDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	projectRoot := t.TempDir()
	outsideRoot := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=test@test", "-c", "user.name=test"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git -C %s %v: %v\n%s", dir, args, err, out)
		}
	}
	run(outsideRoot, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(outsideRoot, "secret.txt"), []byte("outside committed secret\n"), 0o644))
	run(outsideRoot, "add", "secret.txt")
	run(outsideRoot, "commit", "-q", "-m", "secret")
	require.NoError(t, os.Symlink(outsideRoot, filepath.Join(projectRoot, "linked-outside")))

	svc := newTestService(t, projectRoot, fileinfra.NewLocalRepository())
	result, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/linked-outside/secret.txt"})
	require.NoError(t, err)
	require.False(t, result.Tracked)
	require.Empty(t, result.Content)
}

func TestBaselineOnRemoteLocationIsUnsupportedNotError(t *testing.T) {
	repo := &spyFileRepository{}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})
	result, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/README.md"})
	require.NoError(t, err, "remote baseline must be a structured answer, not an error")
	require.NotEmpty(t, result.Unsupported)
	require.False(t, result.Tracked)
}

// baselinerRepo wraps spyFileRepository with the optional remoteGitBaseliner
// capability so the service's remote-baseline routing can be exercised.
type baselinerRepo struct {
	*spyFileRepository
	result filedomain.GitBaseline
	err    error
	dir    string
	name   string
	called bool
}

func (r *baselinerRepo) GitBaseline(_ context.Context, _ filedomain.Reference, dir, name string) (filedomain.GitBaseline, error) {
	r.called = true
	r.dir = dir
	r.name = name
	return r.result, r.err
}

func TestBaselineRemoteUnsupportedWhenRepoLacksCapability(t *testing.T) {
	// Plain spyFileRepository does NOT implement remoteGitBaseliner.
	svc := newTestServiceWithProjects(t, &spyFileRepository{}, testProject{root: "/srv/app", locationID: "ssh-build"})
	res, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/main.go"})
	require.NoError(t, err)
	require.NotEmpty(t, res.Unsupported)
	require.False(t, res.Tracked)
}

func TestBaselineRemoteDegradesWhenCapabilityOff(t *testing.T) {
	repo := &baselinerRepo{spyFileRepository: &spyFileRepository{}, err: filedomain.ErrGitBaselineNotPermitted}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})
	res, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/main.go"})
	require.NoError(t, err)
	require.True(t, repo.called)
	require.Contains(t, res.Unsupported, "Locations page")
	require.False(t, res.Tracked)
}

func TestBaselineRemoteReturnsTrackedContent(t *testing.T) {
	repo := &baselinerRepo{spyFileRepository: &spyFileRepository{}, result: filedomain.GitBaseline{Tracked: true, Bytes: []byte("package main\n\nfunc main() {}\n")}}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})
	res, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/cmd/main.go"})
	require.NoError(t, err)
	require.True(t, res.Tracked)
	require.Equal(t, "package main\n\nfunc main() {}\n", res.Content)
	// The service passes the file's own directory + bare name to the baseliner.
	require.Equal(t, "main.go", repo.name)
	require.Contains(t, repo.dir, "/cmd")
}

func TestBaselineRemoteUntrackedDegrades(t *testing.T) {
	repo := &baselinerRepo{spyFileRepository: &spyFileRepository{}, result: filedomain.GitBaseline{Tracked: false}}
	svc := newTestServiceWithProjects(t, repo, testProject{root: "/srv/app", locationID: "ssh-build"})
	res, err := svc.Baseline(context.Background(), GetInput{ProjectID: "project-test-0", Path: "/project/new.go"})
	require.NoError(t, err)
	require.False(t, res.Tracked)
	require.Empty(t, res.Unsupported)
}
