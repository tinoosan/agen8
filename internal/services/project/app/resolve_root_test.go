package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
)

// ResolveRoot is the read-time bridge that lets a project's filesystem location
// follow wherever it is currently checked out. The stored project.root is only a
// seed; the live root is the most-recently-seen active workspace in the
// project's own location. These tests pin that contract: most-recent active
// workspace wins, removed and foreign-location workspaces are ignored, and with
// nothing to consult we fall back to the seed.

func seedProject(t *testing.T, svc *Service, ctx context.Context, root string) project.Project {
	t.Helper()
	proj, err := svc.CreateProject(ctx, CreateProjectInput{Root: root, Title: "Seed"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return proj
}

func mustCreateWorkspace(t *testing.T, svc *Service, ctx context.Context, ws workspace.Record) {
	t.Helper()
	if err := svc.workspaces.Create(ctx, ws); err != nil {
		t.Fatalf("create workspace %s: %v", ws.ID, err)
	}
}

func at(base time.Time, d time.Duration) *time.Time {
	t := base.Add(d)
	return &t
}

func TestResolveRootFallsBackToStoredRootWhenNoWorkspace(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj := seedProject(t, svc, ctx, "/seed/app")

	if got := svc.ResolveRoot(ctx, proj); got != proj.Root() {
		t.Fatalf("ResolveRoot=%q want stored seed %q", got, proj.Root())
	}
}

func TestResolveRootPrefersMostRecentActiveWorkspace(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj := seedProject(t, svc, ctx, "/seed/app")
	loc := string(proj.LocationID())
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Two active workspaces in the project's location. The moved one was seen
	// most recently, so it must win over both the seed and the older workspace.
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-old", ProjectID: string(proj.ID()), LocationID: loc,
		Root: "/old/app", LifecycleState: workspace.LifecycleActive,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, time.Hour),
	})
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-moved", ProjectID: string(proj.ID()), LocationID: loc,
		Root: "/moved/app", LifecycleState: workspace.LifecycleActive,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, 48*time.Hour),
	})

	if got := svc.ResolveRoot(ctx, proj); got != "/moved/app" {
		t.Fatalf("ResolveRoot=%q want most-recent active workspace /moved/app", got)
	}
}

func TestResolveRootIgnoresRemovedAndForeignLocationWorkspaces(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj := seedProject(t, svc, ctx, "/seed/app")
	loc := string(proj.LocationID())
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// A removed workspace and a foreign-location workspace are both newer than the
	// only valid candidate, yet neither may win: a removed link is gone, and a
	// root from another location is not interpretable here.
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-removed", ProjectID: string(proj.ID()), LocationID: loc,
		Root: "/removed/app", LifecycleState: workspace.LifecycleRemoved,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, 72*time.Hour),
	})
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-foreign", ProjectID: string(proj.ID()), LocationID: "remote-box",
		Root: "/foreign/app", LifecycleState: workspace.LifecycleActive,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, 96*time.Hour),
	})
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-valid", ProjectID: string(proj.ID()), LocationID: loc,
		Root: "/valid/app", LifecycleState: workspace.LifecycleActive,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, time.Hour),
	})

	if got := svc.ResolveRoot(ctx, proj); got != "/valid/app" {
		t.Fatalf("ResolveRoot=%q want same-location active /valid/app", got)
	}
}

func TestResolveRootFallsBackWhenOnlyInvalidWorkspaces(t *testing.T) {
	t.Parallel()
	clock := &mutableClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)}
	svc := newProjectServiceWithClock(t, clock)
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj := seedProject(t, svc, ctx, "/seed/app")
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Only a foreign-location workspace exists. With no usable candidate, the
	// effective root must be the stored seed rather than the foreign root.
	mustCreateWorkspace(t, svc, ctx, workspace.Record{
		ID: "ws-foreign", ProjectID: string(proj.ID()), LocationID: "remote-box",
		Root: "/foreign/app", LifecycleState: workspace.LifecycleActive,
		LinkedAt: base, UpdatedAt: base, LastSeenAt: at(base, time.Hour),
	})

	if got := svc.ResolveRoot(ctx, proj); got != proj.Root() {
		t.Fatalf("ResolveRoot=%q want stored seed %q", got, proj.Root())
	}
}
