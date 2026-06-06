package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/workspace"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type acceptingConfigValidator struct{}

func (acceptingConfigValidator) ValidateConfig(_, _, _ string) error { return nil }

func (acceptingConfigValidator) ValidateRuntimeConfig(_, _, _, _, _ string) error {
	return nil
}

func (acceptingConfigValidator) DefaultPermissionMode(harnessKind string) string {
	return harnessKind + "/default"
}

func (acceptingConfigValidator) CompatibilityPermissionMode(harnessKind string) string {
	return harnessKind + "/default"
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) error { return nil }

// fakeLinkTokenIssuer stands in for the auth-backed issuer. It records the last
// request so tests can assert the ownership-verified binding flowed through, and
// echoes a wlt_-prefixed token. Set err to exercise the failure path.
type fakeLinkTokenIssuer struct {
	last  LinkTokenRequest
	calls int
	err   error
}

func (f *fakeLinkTokenIssuer) IssueLinkToken(_ context.Context, req LinkTokenRequest) (LinkTokenIssued, error) {
	f.calls++
	f.last = req
	if f.err != nil {
		return LinkTokenIssued{}, f.err
	}
	token := "wlt_" + req.ProjectID + "secret"
	prefix := token
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return LinkTokenIssued{
		ID:        "link_token_" + req.ProjectID,
		Prefix:    prefix,
		Token:     token,
		UserID:    req.UserID,
		ProjectID: req.ProjectID,
		Label:     req.Label,
		ExpiresAt: req.ExpiresAt,
		CreatedAt: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
	}, nil
}

func TestRegisterMCPContextUpdatesExistingMemberDisplayName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	first, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "codex",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	if first.DisplayName != "codex" {
		t.Fatalf("first display=%q want codex", first.DisplayName)
	}

	second, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "backend engineer",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("second register: %v", err)
	}
	if second.MemberID != first.MemberID {
		t.Fatalf("member id changed from %q to %q", first.MemberID, second.MemberID)
	}
	if second.DisplayName != "backend engineer" {
		t.Fatalf("second display=%q want backend engineer", second.DisplayName)
	}

	memberRecord, err := service.GetMember(caller.ContextWithCaller(ctx, caller.Caller{UserID: "user-1"}), member.ID(second.MemberID))
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if memberRecord.DisplayName != "backend engineer" {
		t.Fatalf("stored display=%q want backend engineer", memberRecord.DisplayName)
	}
}

func TestRegisterMCPContextRehomesLegacyLocalMemberToTokenUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	legacy, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "local",
		ProjectRoot: root,
		DisplayName: "codex",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("legacy register: %v", err)
	}

	registered, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Codex backend engineer",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("real user register: %v", err)
	}
	if registered.MemberID != legacy.MemberID {
		t.Fatalf("member id changed from %q to %q", legacy.MemberID, registered.MemberID)
	}

	resolved, err := service.ResolveMCPContext(ctx, ResolveMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("resolve rehomed member: %v", err)
	}
	if string(resolved.ID) != registered.MemberID {
		t.Fatalf("resolved member=%q want %q", resolved.ID, registered.MemberID)
	}
	if resolved.UserID != "user-1" {
		t.Fatalf("resolved user=%q want user-1", resolved.UserID)
	}
	if resolved.DisplayName != "Codex backend engineer" {
		t.Fatalf("resolved display=%q want Codex backend engineer", resolved.DisplayName)
	}

	rehomedProject, err := service.GetProject(caller.ContextWithCaller(ctx, caller.Caller{UserID: "user-1"}), types.ProjectID(resolved.ProjectID))
	if err != nil {
		t.Fatalf("get rehomed project: %v", err)
	}
	if rehomedProject.UserID() != "user-1" {
		t.Fatalf("project user=%q want user-1", rehomedProject.UserID())
	}
}

func TestRegisterMCPContextRecordsWorkspace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	result, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	workspaces, err := service.ListWorkspaces(ctx, workspace.Filter{ProjectID: result.ProjectID})
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspaces=%d want 1", len(workspaces))
	}
	ws := workspaces[0]
	if ws.ProjectID != result.ProjectID {
		t.Fatalf("workspace project=%q want %q", ws.ProjectID, result.ProjectID)
	}
	if ws.Root != root {
		t.Fatalf("workspace root=%q want %q", ws.Root, root)
	}
	if ws.UserID != "user-1" {
		t.Fatalf("workspace user=%q want user-1", ws.UserID)
	}
}

func TestRegisterMCPContextBoundProjectOverridesCallerAssertedProjectID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	rootA := filepath.Join(t.TempDir(), "repo-a")
	rootB := filepath.Join(t.TempDir(), "repo-b")

	// Stand up two distinct projects via the path-hash fallback.
	bound, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: rootA,
		HarnessKind: "codex",
		SessionID:   "session-a",
	})
	if err != nil {
		t.Fatalf("register bound project: %v", err)
	}
	spoofed, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: rootB,
		HarnessKind: "codex",
		SessionID:   "session-b",
	})
	if err != nil {
		t.Fatalf("register spoofed project: %v", err)
	}
	if bound.ProjectID == spoofed.ProjectID {
		t.Fatalf("expected distinct project ids, both=%q", bound.ProjectID)
	}

	// The session is bound (server-side) to bound.ProjectID, but the caller
	// asserts spoofed.ProjectID. The unspoofable binding must win.
	result, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:          "ak_test_token",
		UserID:         "user-1",
		BoundProjectID: bound.ProjectID,
		ProjectID:      spoofed.ProjectID,
		ProjectRoot:    rootB,
		HarnessKind:    "codex",
		SessionID:      "session-c",
	})
	if err != nil {
		t.Fatalf("register with binding: %v", err)
	}
	if result.ProjectID != bound.ProjectID {
		t.Fatalf("bound project not honored: result=%q want %q (caller asserted %q)", result.ProjectID, bound.ProjectID, spoofed.ProjectID)
	}
}

func TestRegisterMCPContextPathHashFallbackForUnmarkedFolder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "unmarked")

	result, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	want := string(ProjectIDForLocationRoot("local", root))
	if result.ProjectID != want {
		t.Fatalf("fallback project=%q want %q", result.ProjectID, want)
	}
}

func TestRegisterMCPContextRequiresSomeProjectBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)

	_, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err == nil {
		t.Fatal("expected missing marker, project_id, and project_root to fail loudly")
	}
}

func TestUpsertWorkspaceIsIdentityStable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	first, err := service.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		ProjectID:  "proj-1",
		UserID:     "user-1",
		LocationID: "local",
		Root:       root,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := service.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		ProjectID:  "proj-1",
		UserID:     "user-1",
		LocationID: "local",
		Root:       root,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("workspace id changed from %q to %q", first.ID, second.ID)
	}

	workspaces, err := service.ListWorkspaces(ctx, workspace.Filter{ProjectID: "proj-1"})
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspaces=%d want 1 (identity-stable upsert must not duplicate)", len(workspaces))
	}
}

func TestUpsertWorkspaceRequiresProjectAndRoot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)

	if _, err := service.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		Root: filepath.Join(t.TempDir(), "repo"),
	}); err == nil {
		t.Fatal("expected missing project id to fail loudly")
	}
	if _, err := service.UpsertWorkspace(ctx, UpsertWorkspaceParams{
		ProjectID: "proj-1",
	}); err == nil {
		t.Fatal("expected missing root to fail loudly")
	}
}

func newProjectServiceForMCPContextTest(t *testing.T) *Service {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	projects, err := projectinfra.NewRepository(handle)
	if err != nil {
		t.Fatalf("new project repo: %v", err)
	}
	members, err := projectinfra.NewMemberRepository(handle)
	if err != nil {
		t.Fatalf("new member repo: %v", err)
	}
	workspaces, err := projectinfra.NewWorkspaceRepository(handle)
	if err != nil {
		t.Fatalf("new workspace repo: %v", err)
	}
	service, err := NewService(Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		LinkTokens: &fakeLinkTokenIssuer{},
		Clock:      fixedClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)},
		Caller:     caller.ContextResolver{},
		Configs:    acceptingConfigValidator{},
		Events:     noopPublisher{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}
