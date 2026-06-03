package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type memRepo struct {
	members map[string]member.Record
}

func newMemRepo() *memRepo { return &memRepo{members: make(map[string]member.Record)} }

func (r *memRepo) Create(_ context.Context, m member.Record) error {
	r.members[string(m.ID)] = m
	return nil
}

func (r *memRepo) Get(_ context.Context, id string) (member.Record, error) {
	m, ok := r.members[id]
	if !ok {
		return member.Record{}, member.ErrNotFound
	}
	return m, nil
}

func (r *memRepo) List(_ context.Context, filter member.Filter) ([]member.Record, error) {
	var out []member.Record
	for _, m := range r.members {
		if filter.SpaceID != "" && string(m.SpaceID) != filter.SpaceID {
			continue
		}
		if filter.ProjectID != "" && string(m.ProjectID) != filter.ProjectID {
			continue
		}
		if filter.UserID != "" && m.UserID != filter.UserID {
			continue
		}
		if filter.MemberType != "" && m.MemberType != filter.MemberType {
			continue
		}
		if filter.LifecycleState != "" && m.LifecycleState != filter.LifecycleState {
			continue
		}
		out = append(out, m)
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out, nil
}

func (r *memRepo) Update(_ context.Context, m member.Record) error {
	r.members[string(m.ID)] = m
	return nil
}

func fixedClock() member.Clock {
	return member.FixedClock{T: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)}
}

type testConfigValidator struct{}

func (testConfigValidator) ValidateConfig(harnessKind, model, effort string) error {
	if harnessKind == "" {
		return fmt.Errorf("harnessKind is required")
	}
	if model == "" {
		return fmt.Errorf("model is required")
	}
	if effort == "" {
		return fmt.Errorf("effort is required")
	}
	if harnessKind == "claude-cli" && strings.HasPrefix(model, "gpt-") {
		return fmt.Errorf("unsupported model")
	}
	return nil
}

type testEventPublisher struct {
	events []any
	err    error
}

func (p *testEventPublisher) Publish(_ string, event any) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}

func newTestService() (*Service, *memRepo) {
	svc, members, _ := newTestServiceWithEvents()
	return svc, members
}

func newTestServiceWithEvents() (*Service, *memRepo, *testEventPublisher) {
	spaces := newServiceTestRepo(
		domain.SpaceRecord{ID: "space-1", UserID: "user-1", Status: domain.SpaceStatusOpen},
		domain.SpaceRecord{ID: "space-2", UserID: "user-1", Status: domain.SpaceStatusOpen},
	)
	members := newMemRepo()
	events := &testEventPublisher{}
	svc, err := NewService(spaces, members, domain.FixedClock{T: fixedClock().Now()}, caller.ContextResolver{}, testConfigValidator{}, events, nil)
	if err != nil {
		panic(err)
	}
	return svc, members, events
}

func testMemberCtx() context.Context {
	return caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"})
}

func TestRegister_Worker(t *testing.T) {
	svc, _, events := newTestServiceWithEvents()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, member.TypeWorker, result.GrantedMemberType)
	assert.Equal(t, member.TypeWorker, result.Member.MemberType)
	assert.Equal(t, member.LifecycleActive, result.Member.LifecycleState)
	assert.NotEmpty(t, result.Member.ID)
	assert.NotEmpty(t, result.Member.ChannelID)
	require.Len(t, events.events, 1)
	event, ok := events.events[0].(eventbus.SpaceMemberLifecycleEvent)
	require.True(t, ok)
	assert.Equal(t, eventbus.SpaceMemberEventRegistered, event.EventType)
	assert.Equal(t, string(result.Member.ID), event.MemberID)
	assert.Equal(t, result.Member.HarnessKind, event.HarnessKind)
}

func TestRegister_FirstCoordinator_GrantsCoordinator(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeCoordinator,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, member.TypeCoordinator, result.GrantedMemberType)
	assert.Equal(t, member.TypeCoordinator, result.Member.MemberType)
}

func TestRegister_CoordinatorWithExistingWorker_GrantsCoordinator(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeCoordinator,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, member.TypeCoordinator, result.GrantedMemberType)
}

func TestRegister_SecondCoordinator_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeCoordinator,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	_, err = svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeCoordinator,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active coordinator already exists")
}

func TestRegister_MissingSpaceID_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RegisterMember(testMemberCtx(), member.Record{
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spaceId is required")
}

func TestRegister_MissingHarnessKind_ReturnsError(t *testing.T) {
	svc, _ := newCatalogService()
	_, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID: "space-1",
		Model:   "claude-opus-4-7",
		Effort:  "high",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harnessKind is required")
}

func TestRegister_DefaultDisplayName(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, "worker", result.Member.DisplayName)
}

func TestRegister_CustomDisplayName(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		DisplayName: "Backend Agent",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, "Backend Agent", result.Member.DisplayName)
}

func TestGet_Exists(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	got, err := svc.GetMember(testMemberCtx(), result.Member.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Member.ID, got.ID)
}

func TestDisplayName(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		DisplayName: "Backend Agent",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	name, err := svc.DisplayName(testMemberCtx(), result.Member.ID)
	require.NoError(t, err)
	assert.Equal(t, "Backend Agent", name)
}

func TestDisplayName_NotFound(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.DisplayName(testMemberCtx(), "nonexistent")
	require.Error(t, err)
}

func TestGet_NotFound(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.GetMember(testMemberCtx(), "nonexistent")
	require.Error(t, err)
}

func TestGet_EmptyID_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.GetMember(testMemberCtx(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memberId is required")
}

func TestList_FilterBySpace(t *testing.T) {
	svc, _ := newTestService()
	for _, spaceID := range []string{"space-1", "space-1", "space-2"} {
		_, err := svc.RegisterMember(testMemberCtx(), member.Record{
			SpaceID:     spaceID,
			MemberType:  member.TypeWorker,
			HarnessKind: "claude-cli",
			Model:       "claude-opus-4-7",
			Effort:      "high",
		})
		require.NoError(t, err)
	}

	members, err := svc.ListMembers(testMemberCtx(), member.Filter{SpaceID: "space-1"})
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestList_NoFilter_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.ListMembers(testMemberCtx(), member.Filter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spaceId, projectId, or userId is required")
}

func TestUpdateConfig(t *testing.T) {
	svc, _, events := newTestServiceWithEvents()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateMemberConfig(testMemberCtx(), result.Member.ID, "gpt-5.5", "high", "codex")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5", updated.Model)
	assert.Equal(t, "high", updated.Effort)
	assert.Equal(t, "codex", updated.HarnessKind)
	require.Len(t, events.events, 2)
	event, ok := events.events[1].(eventbus.SpaceMemberLifecycleEvent)
	require.True(t, ok)
	assert.Equal(t, eventbus.SpaceMemberEventConfigChanged, event.EventType)
	assert.Equal(t, "codex", event.HarnessKind)
}

func TestUpdateConfig_EmptyModel_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	_, err = svc.UpdateMemberConfig(testMemberCtx(), result.Member.ID, "", "low", "codex")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model is required")
}

func TestUpdateConfig_NotFound_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.UpdateMemberConfig(testMemberCtx(), "nonexistent", "gpt-5.5", "high", "codex")
	require.Error(t, err)
}

func TestRemove(t *testing.T) {
	svc, _, events := newTestServiceWithEvents()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	removed, err := svc.RemoveMember(testMemberCtx(), result.Member.ID)
	require.NoError(t, err)
	assert.Equal(t, member.LifecycleRemoved, removed.LifecycleState)
	require.Len(t, events.events, 2)
	event, ok := events.events[1].(eventbus.SpaceMemberLifecycleEvent)
	require.True(t, ok)
	assert.Equal(t, eventbus.SpaceMemberEventRemoved, event.EventType)
	assert.Equal(t, member.LifecycleRemoved, event.LifecycleState)
}

func TestRemove_EmptyID_ReturnsError(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RemoveMember(testMemberCtx(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memberId is required")
}

func newCatalogService() (*Service, *memRepo) {
	return newTestService()
}

func TestRegister_WithCatalog_ValidConfig(t *testing.T) {
	svc, _ := newCatalogService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-7", result.Member.Model)
}

func TestRegister_WithCatalog_CrossHarnessRejected(t *testing.T) {
	svc, _ := newCatalogService()
	_, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "gpt-5.5",
		Effort:      "high",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
}

func TestUpdateConfig_WithCatalog_CrossHarnessRejected(t *testing.T) {
	svc, _ := newCatalogService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	_, err = svc.UpdateMemberConfig(testMemberCtx(), result.Member.ID, "gpt-5.5", "high", "claude-cli")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
}

func TestUpdateConfig_WithCatalog_HarnessSwitch(t *testing.T) {
	svc, _ := newCatalogService()
	result, err := svc.RegisterMember(testMemberCtx(), member.Record{
		SpaceID:     "space-1",
		MemberType:  member.TypeWorker,
		HarnessKind: "claude-cli",
		Model:       "claude-opus-4-7",
		Effort:      "high",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateMemberConfig(testMemberCtx(), result.Member.ID, "gpt-5.5", "high", "codex")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5", updated.Model)
	assert.Equal(t, "codex", updated.HarnessKind)
}
