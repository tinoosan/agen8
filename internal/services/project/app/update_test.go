package app

import (
	"context"
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
