package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
)

// A project is owned by a user. SaveProject - and CreateProject, which delegates
// to it - must refuse to persist a project when no owning UserID can be resolved.
// Otherwise an owner-less project (UserID="") leaks past every later ownership
// gate: requireOwnedProject and link-token minting would both treat it as
// unowned. This guards against re-introducing the silent userID="" fallback that
// previously swallowed the caller-resolution error.
func TestSaveProjectRequiresOwningUser(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})

	// 1. No caller stamped at all: caller resolution fails, so creation is rejected.
	if _, err := svc.CreateProject(context.Background(), CreateProjectInput{Root: "/work/app"}); err == nil {
		t.Fatal("expected CreateProject without a caller to fail loudly")
	}

	// 2. Caller present but member-only (no UserID): resolution succeeds, yet a
	//    member cannot own a project, so creation is still rejected.
	memberOnly := caller.ContextWithCaller(context.Background(), caller.Caller{MemberID: "member-1"})
	if _, err := svc.CreateProject(memberOnly, CreateProjectInput{Root: "/work/app"}); err == nil {
		t.Fatal("expected CreateProject with a member-only caller to fail loudly")
	}

	// Neither rejected attempt may leave a half-written, owner-less project behind.
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	projects, err := svc.ListProjects(owner, project.Filter{})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected no project persisted after rejected creates, got %d", len(projects))
	}
}

// An authenticated user-owner can create a project and is recorded as its owner.
// This is the success-path counterpart to the failure cases above - it proves the
// loud-fail guard does not reject the legitimate ownership flow.
func TestSaveProjectStampsOwningUser(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "App"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if proj.UserID() != "user-owner" {
		t.Fatalf("project UserID=%q want user-owner", proj.UserID())
	}
}

func TestProjectReadsAreScopedToCaller(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})
	ownerA := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-a"})
	ownerB := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-b"})

	projectA, err := svc.CreateProject(ownerA, CreateProjectInput{Root: "/work/a", Title: "A"})
	if err != nil {
		t.Fatalf("create project A: %v", err)
	}
	projectB, err := svc.CreateProject(ownerB, CreateProjectInput{Root: "/work/b", Title: "B"})
	if err != nil {
		t.Fatalf("create project B: %v", err)
	}

	if _, err := svc.GetProject(ownerA, projectB.ID()); err == nil {
		t.Fatal("owner A read owner B's project")
	}
	listed, err := svc.ListProjects(ownerA, project.Filter{})
	if err != nil {
		t.Fatalf("list owner A projects: %v", err)
	}
	if len(listed) != 1 || listed[0].ID() != projectA.ID() {
		t.Fatalf("owner A projects=%v want only %s", listed, projectA.ID())
	}
	if _, err := svc.ArchiveProject(ownerA, projectB.ID()); err == nil {
		t.Fatal("owner A archived owner B's project")
	}
}

// Deletion is a deliberate two-step: a project must be archived before it can be
// permanently removed. Deleting an open project must return ErrNotArchived (a
// client precondition the RPC layer surfaces as invalid-params), NOT a generic
// error the dispatcher would mask as a -32603 internal error. This is the
// regression for the "delete returns internal error" report.
func TestDeleteProjectRequiresArchiveFirst(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "App"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	err = svc.DeleteProject(owner, proj.ID())
	if !errors.Is(err, project.ErrNotArchived) {
		t.Fatalf("delete of an open project: got %v, want ErrNotArchived", err)
	}

	// The project must still be there - a refused delete may not remove anything.
	if _, err := svc.GetProject(owner, proj.ID()); err != nil {
		t.Fatalf("project should survive a refused delete: %v", err)
	}
}

// Once archived, a delete succeeds and the record is gone. Deletion removes only
// the record - it never touches the project's files on disk - so a missing root
// directory is irrelevant to it. This is the success-path counterpart to the
// archive-first guard, and it covers the user's "root already removed" scenario:
// the record is the thing being deleted.
func TestDeleteProjectAfterArchiveRemovesRecord(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(owner, CreateProjectInput{Root: "/work/app", Title: "App"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := svc.ArchiveProject(owner, proj.ID()); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}

	if err := svc.DeleteProject(owner, proj.ID()); err != nil {
		t.Fatalf("delete of an archived project should succeed: %v", err)
	}
	if _, err := svc.GetProject(owner, proj.ID()); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("deleted project should be gone: got %v, want ErrNotFound", err)
	}
}

// Deleting a project that does not exist returns ErrNotFound (a client error),
// not a masked internal error.
func TestDeleteProjectNotFound(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithIssuer(t, &fakeLinkTokenIssuer{})
	owner := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	err := svc.DeleteProject(owner, types.ProjectID("does-not-exist"))
	if !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("delete of a missing project: got %v, want ErrNotFound", err)
	}
}
