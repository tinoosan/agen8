package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
)

// mutableClock lets a test advance time between create and update so the two
// operations stamp distinct timestamps. This is what makes the createdAt
// preservation assertion meaningful: with a fixed clock, created and updated
// times coincide and a clobbering bug would hide.
type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }

func newProjectServiceWithClock(t *testing.T, clock Clock) *Service {
	t.Helper()
	projects, members, workspaces := openProjectReposForTest(t)
	service, err := NewService(Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		LinkTokens: &fakeLinkTokenIssuer{},
		Clock:      clock,
		Caller:     caller.ContextResolver{},
		Configs:    acceptingConfigValidator{},
		Events:     noopPublisher{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

// UpdateProject is the user-facing rename/recolor path. It must change title and
// customization and bump updatedAt, while leaving identity (id, root, status,
// owner) and createdAt exactly as they were. This is the guarantee SaveProject
// cannot make - it re-owns the caller and stamps createdAt=now - so the update
// path needs its own coverage.
func TestUpdateProjectRenamesAndPreservesIdentity(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: created}
	svc := newProjectServiceWithClock(t, clock)
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "Old"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Advance the clock so update stamps a strictly later updatedAt.
	updated := created.Add(48 * time.Hour)
	clock.now = updated

	newTitle := "New Name"
	got, err := svc.UpdateProject(owner, UpdateProjectInput{
		ProjectID:     proj.ID(),
		Title:         &newTitle,
		Customization: &project.Customization{Icon: "rocket", Color: "#abcdef"},
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	if got.Title() != "New Name" {
		t.Fatalf("Title=%q want New Name", got.Title())
	}
	if c := got.Customization(); c == nil || c.Icon != "rocket" || c.Color != "#abcdef" {
		t.Fatalf("Customization=%+v want rocket/#abcdef", c)
	}
	// Identity is frozen across the edit.
	if got.ID() != proj.ID() {
		t.Fatalf("ID changed: %q -> %q", proj.ID(), got.ID())
	}
	if got.Root() != proj.Root() {
		t.Fatalf("Root changed: %q -> %q", proj.Root(), got.Root())
	}
	if got.UserID() != "user-owner" {
		t.Fatalf("owner changed: %q", got.UserID())
	}
	if got.Status() != proj.Status() {
		t.Fatalf("status changed: %q -> %q", proj.Status(), got.Status())
	}
	if !got.CreatedAt().Equal(created) {
		t.Fatalf("CreatedAt=%s want preserved %s", got.CreatedAt(), created)
	}
	if !got.UpdatedAt().Equal(updated) {
		t.Fatalf("UpdatedAt=%s want bumped %s", got.UpdatedAt(), updated)
	}
}

// A nil field means "leave alone": recoloring must not wipe the title, and the
// pointer-per-field contract is what guarantees it.
func TestUpdateProjectLeavesNilFieldsUnchanged(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "Keep Me"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	got, err := svc.UpdateProject(owner, UpdateProjectInput{
		ProjectID:     proj.ID(),
		Customization: &project.Customization{Icon: "flag"},
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if got.Title() != "Keep Me" {
		t.Fatalf("Title=%q want untouched Keep Me", got.Title())
	}
	if c := got.Customization(); c == nil || c.Icon != "flag" {
		t.Fatalf("Customization=%+v want flag", c)
	}
}

// Only the owner may edit a project. A non-owner attempt must fail and must not
// mutate the stored record.
func TestUpdateProjectRejectsNonOwner(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "Owned"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	intruder := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-intruder"})
	hijack := "Hijacked"
	if _, err := svc.UpdateProject(intruder, UpdateProjectInput{ProjectID: proj.ID(), Title: &hijack}); err == nil {
		t.Fatal("expected non-owner update to be rejected")
	}

	// The stored record must be untouched by the rejected attempt.
	after, err := svc.GetProject(owner, proj.ID())
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if after.Title() != "Owned" {
		t.Fatalf("Title=%q want unchanged Owned after rejected update", after.Title())
	}
}

func TestRelocateProjectPreservesIdentityAndRejectsRootCollision(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	first, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/first", Title: "First"})
	if err != nil {
		t.Fatalf("CreateProject first: %v", err)
	}
	if _, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/taken", Title: "Taken"}); err != nil {
		t.Fatalf("CreateProject taken: %v", err)
	}

	clock.now = clock.now.Add(time.Hour)
	relocated, err := svc.RelocateProject(owner, RelocateProjectInput{ProjectID: first.ID(), Root: "/work/renamed"})
	if err != nil {
		t.Fatalf("RelocateProject: %v", err)
	}
	if relocated.ID() != first.ID() || relocated.Root() != "/work/renamed" {
		t.Fatalf("relocated project id=%q root=%q", relocated.ID(), relocated.Root())
	}
	if relocated.Title() != first.Title() || relocated.UserID() != first.UserID() || !relocated.CreatedAt().Equal(first.CreatedAt()) {
		t.Fatalf("relocation changed non-root identity: before=%+v after=%+v", first.Record(), relocated.Record())
	}

	_, err = svc.RelocateProject(owner, RelocateProjectInput{ProjectID: first.ID(), Root: "/work/taken"})
	if !errors.Is(err, project.ErrRootInUse) {
		t.Fatalf("collision error=%v want ErrRootInUse", err)
	}
	after, err := svc.GetProject(owner, first.ID())
	if err != nil {
		t.Fatalf("GetProject after collision: %v", err)
	}
	if after.Root() != "/work/renamed" {
		t.Fatalf("collision partially changed root to %q", after.Root())
	}
}

func TestRelocateProjectRejectsNonOwner(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithClock(t, &mutableClock{now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	created, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/original"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	intruder := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-intruder"})
	validated := false
	if _, err := svc.RelocateProject(intruder, RelocateProjectInput{
		ProjectID: created.ID(),
		Root:      "/work/hijacked",
		Validate: func(context.Context, string, string) error {
			validated = true
			return nil
		},
	}); err == nil {
		t.Fatal("expected non-owner relocation to fail")
	}
	if validated {
		t.Fatal("non-owner reached project root validation")
	}
	after, err := svc.GetProject(owner, created.ID())
	if err != nil || after.Root() != "/work/original" {
		t.Fatalf("rejected relocation changed project: root=%q err=%v", after.Root(), err)
	}
}

func TestRelocateProjectKeepsExplicitWorkspacesAndDropsTheOldCanonicalRoot(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithClock(t, &mutableClock{now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	created, err := svc.CreateProject(owner, CreateProjectInput{Root: "/repo/old"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	linked, err := svc.UpsertWorkspace(owner, UpsertWorkspaceParams{
		ProjectID:  string(created.ID()),
		UserID:     "user-owner",
		LocationID: "local",
		Root:       "/repo/worktree",
		Machine:    "laptop",
	})
	if err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	relocated, err := svc.RelocateProject(owner, RelocateProjectInput{ProjectID: created.ID(), Root: "/repo/new"})
	if err != nil {
		t.Fatalf("RelocateProject: %v", err)
	}
	resolved, err := svc.ResolveBoundWorkspaceRoot(owner, string(created.ID()), "user-owner", "local", linked.ID.String())
	if err != nil || resolved != "/repo/worktree" {
		t.Fatalf("explicit workspace resolved=%q err=%v", resolved, err)
	}
	roots, err := svc.ActiveWorkspaceRoots(owner, relocated)
	if err != nil {
		t.Fatalf("ActiveWorkspaceRoots: %v", err)
	}
	joined := strings.Join(roots, "\n")
	if !strings.Contains(joined, "/repo/new") || !strings.Contains(joined, "/repo/worktree") || strings.Contains(joined, "/repo/old") {
		t.Fatalf("active roots after relocation=%v", roots)
	}
}
