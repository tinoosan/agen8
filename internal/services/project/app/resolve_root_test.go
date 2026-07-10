package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
)

func seedProject(t *testing.T, ctx context.Context, svc *Service, root string) project.Project {
	t.Helper()
	proj, err := svc.CreateProject(ctx, CreateProjectInput{Root: root, Title: "Seed"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return proj
}

func mustCreateWorkspace(t *testing.T, ctx context.Context, svc *Service, ws workspace.Record) {
	t.Helper()
	if err := svc.workspaces.Create(ctx, ws); err != nil {
		t.Fatalf("create workspace %s: %v", ws.ID, err)
	}
}

func TestActiveWorkspaceRootsIncludesStableRootAndOwnedWorkspaces(t *testing.T) {
	t.Parallel()
	svc := newProjectServiceWithClock(t, &mutableClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)})
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	proj := seedProject(t, ctx, svc, "/seed/app")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	for _, item := range []workspace.Record{
		{ID: "ws-worktree", ProjectID: string(proj.ID()), UserID: "user-owner", LocationID: string(proj.LocationID()), Root: "/worktree/app", LifecycleState: workspace.LifecycleActive, LinkedAt: base, UpdatedAt: base},
		{ID: "ws-duplicate", ProjectID: string(proj.ID()), UserID: "user-owner", LocationID: string(proj.LocationID()), Root: "/seed/app", LifecycleState: workspace.LifecycleActive, LinkedAt: base, UpdatedAt: base},
		{ID: "ws-removed", ProjectID: string(proj.ID()), UserID: "user-owner", LocationID: string(proj.LocationID()), Root: "/removed/app", LifecycleState: workspace.LifecycleRemoved, LinkedAt: base, UpdatedAt: base},
		{ID: "ws-foreign-location", ProjectID: string(proj.ID()), UserID: "user-owner", LocationID: "remote", Root: "/remote/app", LifecycleState: workspace.LifecycleActive, LinkedAt: base, UpdatedAt: base},
		{ID: "ws-foreign-user", ProjectID: string(proj.ID()), UserID: "user-other", LocationID: string(proj.LocationID()), Root: "/other/app", LifecycleState: workspace.LifecycleActive, LinkedAt: base, UpdatedAt: base},
	} {
		mustCreateWorkspace(t, ctx, svc, item)
	}

	roots, err := svc.ActiveWorkspaceRoots(ctx, proj)
	if err != nil {
		t.Fatalf("ActiveWorkspaceRoots: %v", err)
	}
	want := map[string]bool{"/seed/app": true, "/worktree/app": true}
	if len(roots) != len(want) {
		t.Fatalf("roots=%v want stable root and owned active worktree", roots)
	}
	for _, root := range roots {
		if !want[root] {
			t.Fatalf("unexpected active root %q in %v", root, roots)
		}
	}
}

func TestActiveWorkspaceRootsReturnsStableRootWithoutWorkspaceRepository(t *testing.T) {
	proj, err := project.New(project.NewInput{
		ID: "project-1", Root: "/seed/app", UserID: "user-owner", CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	roots, err := (*Service)(nil).ActiveWorkspaceRoots(context.Background(), proj)
	if err != nil || len(roots) != 1 || roots[0] != "/seed/app" {
		t.Fatalf("roots=%v err=%v", roots, err)
	}
}
