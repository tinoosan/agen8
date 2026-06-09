package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
	projectinfra "github.com/tinoosan/agen8/internal/services/project/infra"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func TestCreateLinkTokenOwnerMintsBoundToken(t *testing.T) {
	t.Parallel()
	issuer := &fakeLinkTokenIssuer{}
	svc := newProjectServiceWithIssuer(t, issuer)
	ownerCtx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	proj, err := svc.CreateProject(ownerCtx, CreateProjectInput{Root: "/work/app", Title: "App"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issued, err := svc.CreateLinkToken(ownerCtx, CreateLinkTokenInput{ProjectID: proj.ID(), Label: "laptop"})
	if err != nil {
		t.Fatalf("CreateLinkToken: %v", err)
	}
	if !strings.HasPrefix(issued.Token, "wlt_") {
		t.Fatalf("token=%q want wlt_ prefix", issued.Token)
	}
	if issued.ProjectID != string(proj.ID()) {
		t.Fatalf("issued.ProjectID=%q want %q", issued.ProjectID, proj.ID())
	}
	if issued.UserID != "user-owner" {
		t.Fatalf("issued.UserID=%q want user-owner", issued.UserID)
	}
	// The issuer must receive the ownership-verified binding, not raw input.
	if issuer.calls != 1 {
		t.Fatalf("issuer.calls=%d want 1", issuer.calls)
	}
	if issuer.last.UserID != "user-owner" || issuer.last.ProjectID != string(proj.ID()) {
		t.Fatalf("issuer.last=%+v want user-owner + %q", issuer.last, proj.ID())
	}
	if issuer.last.Label != "laptop" {
		t.Fatalf("issuer.last.Label=%q want laptop", issuer.last.Label)
	}
}

func TestCreateLinkTokenRejectsNonOwner(t *testing.T) {
	t.Parallel()
	issuer := &fakeLinkTokenIssuer{}
	svc := newProjectServiceWithIssuer(t, issuer)
	ownerCtx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	proj, err := svc.CreateProject(ownerCtx, CreateProjectInput{Root: "/work/app"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	intruderCtx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-intruder"})
	if _, err := svc.CreateLinkToken(intruderCtx, CreateLinkTokenInput{ProjectID: proj.ID()}); err == nil {
		t.Fatal("expected non-owner to be rejected")
	}
	if issuer.calls != 0 {
		t.Fatalf("issuer must not be called for a non-owner, calls=%d", issuer.calls)
	}
}

func TestCreateLinkTokenRequiresProjectID(t *testing.T) {
	t.Parallel()
	issuer := &fakeLinkTokenIssuer{}
	svc := newProjectServiceWithIssuer(t, issuer)
	ownerCtx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})

	if _, err := svc.CreateLinkToken(ownerCtx, CreateLinkTokenInput{ProjectID: ""}); err == nil {
		t.Fatal("expected missing project id to fail loudly")
	}
	if issuer.calls != 0 {
		t.Fatalf("issuer must not be called without a project, calls=%d", issuer.calls)
	}
}

func TestCreateLinkTokenRejectsUnauthenticatedCaller(t *testing.T) {
	t.Parallel()
	issuer := &fakeLinkTokenIssuer{}
	svc := newProjectServiceWithIssuer(t, issuer)

	if _, err := svc.CreateLinkToken(context.Background(), CreateLinkTokenInput{ProjectID: "any-project"}); err == nil {
		t.Fatal("expected unauthenticated caller to be rejected")
	}
	if issuer.calls != 0 {
		t.Fatalf("issuer must not be called without a caller, calls=%d", issuer.calls)
	}
}

func TestCreateLinkTokenPropagatesIssuerError(t *testing.T) {
	t.Parallel()
	issuer := &fakeLinkTokenIssuer{err: errors.New("mint boom")}
	svc := newProjectServiceWithIssuer(t, issuer)
	ownerCtx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-owner"})
	proj, err := svc.CreateProject(ownerCtx, CreateProjectInput{Root: "/work/app"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	_, err = svc.CreateLinkToken(ownerCtx, CreateLinkTokenInput{ProjectID: proj.ID()})
	if err == nil {
		t.Fatal("expected issuer error to propagate")
	}
	if !strings.Contains(err.Error(), "mint boom") {
		t.Fatalf("err=%v want issuer error surfaced", err)
	}
}

func TestNewServiceRequiresLinkTokenIssuer(t *testing.T) {
	t.Parallel()
	projects, members, workspaces := openProjectReposForTest(t)

	_, err := NewService(Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		// LinkTokens deliberately omitted.
		Caller:  caller.ContextResolver{},
		Configs: acceptingConfigValidator{},
		Events:  noopPublisher{},
	})
	if err == nil {
		t.Fatal("expected missing link token issuer to fail loudly")
	}
	if !strings.Contains(err.Error(), "link token issuer") {
		t.Fatalf("err=%v want link token issuer required", err)
	}
}

func newProjectServiceWithIssuer(t *testing.T, issuer LinkTokenIssuer) *Service {
	t.Helper()
	projects, members, workspaces := openProjectReposForTest(t)
	service, err := NewService(Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		LinkTokens: issuer,
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

func openProjectReposForTest(t *testing.T) (project.Repository, member.Repository, workspace.Repository) {
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
	return projects, members, workspaces
}
