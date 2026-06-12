package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
	projectinfra "github.com/tinoosan/agen8/internal/services/project/infra"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
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

// fakeLinkTokenIssuer stands in for the auth-backed link token service. It
// records the last issue request so tests can assert the ownership-verified
// binding flowed through, and echoes a wlt_-prefixed token. The summaries slice
// is what ListLinkTokens returns (keyed by no project — the project service has
// already owner-gated by the time it calls), and revoked records the ids passed
// to RevokeLinkToken. Set err to exercise the issue failure path; listErr and
// revokeErr exercise the list/revoke failure paths.
type fakeLinkTokenIssuer struct {
	last      LinkTokenRequest
	calls     int
	err       error
	summaries []LinkTokenSummary
	listErr   error
	revoked   []string
	revokeErr error
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

func (f *fakeLinkTokenIssuer) ListLinkTokens(_ context.Context, _ string) ([]LinkTokenSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.summaries, nil
}

func (f *fakeLinkTokenIssuer) RevokeLinkToken(_ context.Context, tokenID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = append(f.revoked, tokenID)
	return nil
}

func TestRegisterMCPContextReusesExistingMemberWithoutRenaming(t *testing.T) {
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
	if !second.AlreadyRegistered {
		t.Fatal("second register should report already registered")
	}
	if second.DisplayName != "codex" {
		t.Fatalf("second display=%q want codex", second.DisplayName)
	}

	memberRecord, err := service.GetMember(caller.ContextWithCaller(ctx, caller.Caller{UserID: "user-1"}), member.ID(second.MemberID))
	if err != nil {
		t.Fatalf("get member: %v", err)
	}
	if memberRecord.DisplayName != "codex" {
		t.Fatalf("stored display=%q want codex", memberRecord.DisplayName)
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
	if !registered.AlreadyRegistered {
		t.Fatal("real user register should report already registered")
	}
	if resolved.DisplayName != "codex" {
		t.Fatalf("resolved display=%q want codex", resolved.DisplayName)
	}

	rehomedProject, err := service.GetProject(caller.ContextWithCaller(ctx, caller.Caller{UserID: "user-1"}), types.ProjectID(resolved.ProjectID))
	if err != nil {
		t.Fatalf("get rehomed project: %v", err)
	}
	if rehomedProject.UserID() != "user-1" {
		t.Fatalf("project user=%q want user-1", rehomedProject.UserID())
	}
}

// TestRegisterMCPContextHarnessLabelDriftDoesNotForkMember pins the register-side
// half of the multibinding fix (task-74b59b64). Harness kind seeds a member's id
// (deterministicMemberID), so when one session re-registers under a drifted harness
// label - "claude" one call, "claude-cli" the next - the old code created a SECOND
// member for the same human session. The harness-agnostic resolve then matched both
// and failed with "mcp context resolves to multiple members", blocking even reads.
// Re-registration must reuse the existing member instead of forking a new one.
func TestRegisterMCPContextHarnessLabelDriftDoesNotForkMember(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	first, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Atlas",
		HarnessKind: "claude",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	second, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Atlas",
		HarnessKind: "claude-cli",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("second register (drifted harness label): %v", err)
	}
	if second.MemberID != first.MemberID {
		t.Fatalf("harness label drift forked the member: first=%q second=%q", first.MemberID, second.MemberID)
	}

	members, err := service.members.List(ctx, member.Filter{
		ProjectID:        first.ProjectID,
		NativeSessionRef: "sess-1",
		LifecycleState:   member.LifecycleActive,
	})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("active members for session=%d want 1 (harness label drift must not fork)", len(members))
	}

	// The daemon resolves harness-agnostically (HarnessKind:""). After the drift it
	// must return the one member, not the multi-member error.
	resolved, err := service.ResolveMCPContext(ctx, ResolveMCPContextInput{
		Token:     "ak_test_token",
		UserID:    "user-1",
		ProjectID: first.ProjectID,
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("harness-agnostic resolve after label drift: %v", err)
	}
	if string(resolved.ID) != first.MemberID {
		t.Fatalf("resolved member=%q want %q", resolved.ID, first.MemberID)
	}
}

// TestResolveMCPContextCollapsesHarnessLabelForkSameProject pins the resolve-side
// half of the fix: data already forked by the old code (two active members for the
// same user+project+native ref, differing only by harness label) must heal on read.
// Resolution collapses them to one member of that session instead of erroring.
func TestResolveMCPContextCollapsesHarnessLabelForkSameProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	first, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Atlas",
		HarnessKind: "claude",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Fabricate the fork the old code left behind. The register path no longer creates
	// it, so write the second member directly: same user, project, and native ref, a
	// drifted harness label, and a distinct id.
	authCtx := caller.ContextWithCaller(ctx, caller.Caller{UserID: "user-1"})
	forked, err := service.UpsertExternalHarnessMember(authCtx, UpsertExternalHarnessMemberParams{
		ID:               member.ID("member-forked-clic"),
		UserID:           "user-1",
		ProjectID:        first.ProjectID,
		NativeSessionRef: first.NativeSessionRef,
		DisplayName:      "Atlas",
		HarnessKind:      "claude-cli",
	})
	if err != nil {
		t.Fatalf("fabricate fork: %v", err)
	}

	resolved, err := service.ResolveMCPContext(ctx, ResolveMCPContextInput{
		Token:     "ak_test_token",
		UserID:    "user-1",
		ProjectID: first.ProjectID,
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("resolve over forked members must not error: %v", err)
	}
	if string(resolved.ID) != first.MemberID && resolved.ID != forked.ID {
		t.Fatalf("resolved member=%q is neither fork member (%q / %q)", resolved.ID, first.MemberID, forked.ID)
	}
	if resolved.ProjectID != first.ProjectID {
		t.Fatalf("resolved project=%q want %q", resolved.ProjectID, first.ProjectID)
	}
	if resolved.NativeSessionRef != first.NativeSessionRef {
		t.Fatalf("resolved native ref=%q want %q", resolved.NativeSessionRef, first.NativeSessionRef)
	}
}

// TestResolveMCPContextKeepsLoudErrorOnCrossProjectAmbiguity pins the limit of the
// collapse: it must NOT paper over genuine ambiguity. When one native ref maps to
// members in two different projects (reachable because an api-key session carries no
// bound project, so the lookup is not project-scoped), resolution still cannot know
// which project the caller means and must fail loudly rather than guess an actor
// across a project boundary.
func TestResolveMCPContextKeepsLoudErrorOnCrossProjectAmbiguity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	rootA := filepath.Join(t.TempDir(), "repo-a")
	rootB := filepath.Join(t.TempDir(), "repo-b")

	a, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: rootA,
		DisplayName: "Atlas",
		HarnessKind: "claude",
		SessionID:   "shared-ref",
	})
	if err != nil {
		t.Fatalf("register project A: %v", err)
	}
	b, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: rootB,
		DisplayName: "Atlas",
		HarnessKind: "claude",
		SessionID:   "shared-ref",
	})
	if err != nil {
		t.Fatalf("register project B: %v", err)
	}
	if a.ProjectID == b.ProjectID {
		t.Fatalf("expected distinct projects, both=%q", a.ProjectID)
	}

	// Resolve WITHOUT a project id - the api-key daemon path - must still fail loudly.
	_, err = service.ResolveMCPContext(ctx, ResolveMCPContextInput{
		Token:     "ak_test_token",
		UserID:    "user-1",
		SessionID: "shared-ref",
	})
	if err == nil {
		t.Fatal("expected cross-project ambiguity to fail loudly")
	}
	if !strings.Contains(err.Error(), "resolves to multiple members") {
		t.Fatalf("error=%q want it to mention resolving to multiple members", err)
	}
}

// TestCollapseSessionMembersPrefersEarliestAndGuardsCrossProject pins the collapse
// policy directly, with explicit timestamps so it does not lean on the fixed test
// clock or member-id hash coincidence: the earliest-registered member of a same-session
// fork wins (the original identity that holds pre-fork work), ties break deterministically
// by member id, and a cross-project candidate set still fails loudly.
func TestCollapseSessionMembersPrefersEarliestAndGuardsCrossProject(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	t.Run("empty input is not found", func(t *testing.T) {
		if _, err := collapseSessionMembers(nil); !errors.Is(err, member.ErrNotFound) {
			t.Fatalf("empty input err=%v want member.ErrNotFound", err)
		}
	})

	t.Run("earliest registered wins regardless of input order", func(t *testing.T) {
		oldest := member.Record{ID: "member-old", ProjectID: "proj-1", RegisteredAt: base}
		newer := member.Record{ID: "member-new", ProjectID: "proj-1", RegisteredAt: base.Add(time.Hour)}
		// Newest-first input order proves the choice is by timestamp, not slice position.
		got, err := collapseSessionMembers([]member.Record{newer, oldest})
		if err != nil {
			t.Fatalf("collapse same-project fork: %v", err)
		}
		if got.ID != oldest.ID {
			t.Fatalf("winner=%q want earliest %q", got.ID, oldest.ID)
		}
	})

	t.Run("equal timestamps break by member id deterministically", func(t *testing.T) {
		a := member.Record{ID: "member-aaa", ProjectID: "proj-1", RegisteredAt: base}
		b := member.Record{ID: "member-bbb", ProjectID: "proj-1", RegisteredAt: base}
		got1, err := collapseSessionMembers([]member.Record{a, b})
		if err != nil {
			t.Fatalf("collapse tie ab: %v", err)
		}
		got2, err := collapseSessionMembers([]member.Record{b, a})
		if err != nil {
			t.Fatalf("collapse tie ba: %v", err)
		}
		if got1.ID != got2.ID || got1.ID != a.ID {
			t.Fatalf("tie not deterministic: ab=%q ba=%q want %q", got1.ID, got2.ID, a.ID)
		}
	})

	t.Run("different projects fail loudly", func(t *testing.T) {
		p1 := member.Record{ID: "member-1", ProjectID: "proj-1", RegisteredAt: base}
		p2 := member.Record{ID: "member-2", ProjectID: "proj-2", RegisteredAt: base.Add(time.Hour)}
		_, err := collapseSessionMembers([]member.Record{p1, p2})
		if err == nil || !strings.Contains(err.Error(), "resolves to multiple members") {
			t.Fatalf("cross-project err=%v want it to mention resolving to multiple members", err)
		}
	})
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

func TestRegisterMCPContextWorktreeRootResolvesCanonicalProject(t *testing.T) {
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	mainRoot, worktreeRoot := createGitWorktreeFixture(t)

	mainResult, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: mainRoot,
		HarnessKind: "codex",
		SessionID:   "session-main",
	})
	if err != nil {
		t.Fatalf("register main root: %v", err)
	}
	worktreeResult, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: worktreeRoot,
		HarnessKind: "codex",
		SessionID:   "session-worktree",
	})
	if err != nil {
		t.Fatalf("register worktree root: %v", err)
	}
	if worktreeResult.ProjectID != mainResult.ProjectID {
		t.Fatalf("worktree project=%q want canonical %q", worktreeResult.ProjectID, mainResult.ProjectID)
	}
	if worktreeResult.ProjectRoot != mainRoot {
		t.Fatalf("worktree projectRoot=%q want canonical root %q", worktreeResult.ProjectRoot, mainRoot)
	}
	if worktreeResult.ProjectID != string(ProjectIDForLocationRoot("local", mainRoot)) {
		t.Fatalf("project id=%q want main-root hash", worktreeResult.ProjectID)
	}

	workspaces, err := service.ListWorkspaces(ctx, workspace.Filter{ProjectID: mainResult.ProjectID})
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	roots := map[string]bool{}
	for _, ws := range workspaces {
		roots[ws.Root] = true
	}
	if !roots[mainRoot] || !roots[worktreeRoot] {
		t.Fatalf("workspace roots=%v want main and worktree roots", roots)
	}
}

func TestRegisterMCPContextRegularGitSubdirKeepsPathHashFallback(t *testing.T) {
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	mainRoot := createGitRepoFixture(t)
	subdir := filepath.Join(mainRoot, "tools", "worker")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	result, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: subdir,
		HarnessKind: "codex",
		SessionID:   "session-subdir",
	})
	if err != nil {
		t.Fatalf("register subdir: %v", err)
	}
	want := string(ProjectIDForLocationRoot("local", subdir))
	if result.ProjectID != want {
		t.Fatalf("regular git subdir project=%q want path-hash %q", result.ProjectID, want)
	}
	if result.ProjectRoot != subdir {
		t.Fatalf("regular git subdir root=%q want %q", result.ProjectRoot, subdir)
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

func createGitWorktreeFixture(t *testing.T) (string, string) {
	t.Helper()
	mainRoot := createGitRepoFixture(t)
	worktreeRoot := filepath.Join(t.TempDir(), "repo-worktree")
	runGit(t, mainRoot, "worktree", "add", "-b", "fixture-worktree", worktreeRoot)
	return mainRoot, worktreeRoot
}

func createGitRepoFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary is required for worktree registration tests")
	}
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve repo path: %v", err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "tests@example.com")
	runGit(t, root, "config", "user.name", "Agen8 Tests")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "fixture")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
