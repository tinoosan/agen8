package app

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
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
