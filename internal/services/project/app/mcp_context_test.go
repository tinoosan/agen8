package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
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

func TestRegisterMCPContextUpdatesExistingMemberDisplayName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	service := newProjectServiceForMCPContextTest(t)
	root := filepath.Join(t.TempDir(), "repo")

	first, err := service.RegisterMCPContext(ctx, RegisterMCPContextInput{
		Token:       "agen8-local",
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
		Token:       "agen8-local",
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
		Token:       "agen8-local",
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
		Token:       "agen8-local",
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
		Token:       "agen8-local",
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
	service, err := NewService(Config{
		Projects: projects,
		Members:  members,
		Clock:    fixedClock{now: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)},
		Caller:   caller.ContextResolver{},
		Configs:  acceptingConfigValidator{},
		Events:   noopPublisher{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}
