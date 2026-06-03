package member_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

func TestWrapMember_EmptyLifecycleDefaultsToActive(t *testing.T) {
	m := member.WrapMember(member.Record{})
	assert.Equal(t, member.LifecycleActive, m.Inner().LifecycleState)
}

func TestRemove_ActiveToRemoved(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	m := member.WrapMember(member.Record{LifecycleState: member.LifecycleActive})

	next, err := m.Remove(now)
	require.NoError(t, err)
	assert.Equal(t, member.LifecycleRemoved, next.Inner().LifecycleState)
	assert.Equal(t, now, next.Inner().UpdatedAt)
}

func TestRemove_AlreadyRemoved_ReturnsError(t *testing.T) {
	m := member.WrapMember(member.Record{LifecycleState: member.LifecycleRemoved})
	_, err := m.Remove(time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already removed")
}

func TestSetMemberType_Valid(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	m := member.WrapMember(member.Record{MemberType: member.TypeWorker})

	next, err := m.SetMemberType(member.TypeCoordinator, now)
	require.NoError(t, err)
	assert.Equal(t, member.TypeCoordinator, next.Inner().MemberType)
	assert.Equal(t, now, next.Inner().UpdatedAt)
}

func TestSetMemberType_Invalid_ReturnsError(t *testing.T) {
	m := member.WrapMember(member.Record{MemberType: member.TypeWorker})
	_, err := m.SetMemberType("invalid", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid memberType")
}

func TestUpdateConfig_Valid(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	m := member.WrapMember(member.Record{Model: "old", Effort: "low", HarnessKind: "claude-cli"})

	next, err := m.UpdateConfig("new-model", "high", "codex", now)
	require.NoError(t, err)
	assert.Equal(t, "new-model", next.Inner().Model)
	assert.Equal(t, "high", next.Inner().Effort)
	assert.Equal(t, "codex", next.Inner().HarnessKind)
	assert.Equal(t, now, next.Inner().UpdatedAt)
}

func TestUpdateConfig_EmptyModel_ReturnsError(t *testing.T) {
	m := member.WrapMember(member.Record{})
	_, err := m.UpdateConfig("", "high", "codex", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model is required")
}

func TestUpdateConfig_EmptyEffort_ReturnsError(t *testing.T) {
	m := member.WrapMember(member.Record{})
	_, err := m.UpdateConfig("opus", "", "codex", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "effort is required")
}

func TestUpdateConfig_EmptyHarnessKind_ReturnsError(t *testing.T) {
	m := member.WrapMember(member.Record{})
	_, err := m.UpdateConfig("opus", "high", "", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "harnessKind is required")
}

func TestValidateMemberType(t *testing.T) {
	for _, valid := range []string{member.TypeCoordinator, member.TypeWorker} {
		require.NoError(t, member.ValidateMemberType(valid))
	}
	require.Error(t, member.ValidateMemberType("invalid"))
	require.Error(t, member.ValidateMemberType("lone_coordinator"))
}

func TestValidateLifecycleState(t *testing.T) {
	for _, valid := range []string{member.LifecycleActive, member.LifecycleRemoved} {
		require.NoError(t, member.ValidateLifecycleState(valid))
	}
	require.Error(t, member.ValidateLifecycleState("invalid"))
}

func TestIsCoordinatorType(t *testing.T) {
	assert.True(t, member.IsCoordinatorType(member.TypeCoordinator))
	assert.False(t, member.IsCoordinatorType(member.TypeWorker))
	assert.False(t, member.IsCoordinatorType("lone_coordinator"))
}
