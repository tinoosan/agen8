package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

var testNow = time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

func TestNewSession_Valid(t *testing.T) {
	s, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	assert.Equal(t, "sess-1", s.ID)
	assert.Equal(t, "project-1", s.ProjectID)
	assert.Equal(t, "local", s.LocationID)
	assert.Equal(t, "channel:space-1:member:member-1", s.ChannelID)
	assert.Equal(t, "Worker One", s.DisplayName)
	assert.Equal(t, "/tmp/project-1", s.Workdir)
	assert.Equal(t, "prompt", s.SystemPrompt)
	assert.Equal(t, domain.SessionActive, s.Status)
	assert.Equal(t, testNow, s.ActivatedAt)
	assert.Nil(t, s.DeactivatedAt)
	assert.Empty(t, s.InactiveReason)
	assert.Empty(t, s.InactiveError)
	assert.Equal(t, int64(0), s.TokensIn)
	assert.Equal(t, int64(0), s.TokensOut)
}

func TestNewSession_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*domain.RuntimeContext)
		wantErr string
	}{
		{"empty memberID", func(ctx *domain.RuntimeContext) { ctx.MemberID = "" }, "memberID is required"},
		{"empty projectID", func(ctx *domain.RuntimeContext) { ctx.ProjectID = "" }, "projectID is required"},
		{"empty locationID", func(ctx *domain.RuntimeContext) { ctx.LocationID = "" }, "locationID is required"},
		{"empty spaceID", func(ctx *domain.RuntimeContext) { ctx.SpaceID = "" }, "spaceID is required"},
		{"empty channelID", func(ctx *domain.RuntimeContext) { ctx.ChannelID = "" }, "channelID is required"},
		{"empty displayName", func(ctx *domain.RuntimeContext) { ctx.DisplayName = "" }, "displayName is required"},
		{"empty memberType", func(ctx *domain.RuntimeContext) { ctx.MemberType = "" }, "memberType is required"},
		{"empty lifecycleState", func(ctx *domain.RuntimeContext) { ctx.LifecycleState = "" }, "lifecycleState is required"},
		{"empty kind", func(ctx *domain.RuntimeContext) { ctx.HarnessKind = "" }, "kind is required"},
		{"empty model", func(ctx *domain.RuntimeContext) { ctx.Model = "" }, "model is required"},
		{"empty effort", func(ctx *domain.RuntimeContext) { ctx.Effort = "" }, "effort is required"},
		{"empty systemPrompt", func(ctx *domain.RuntimeContext) { ctx.SystemPrompt = "" }, "systemPrompt is required"},
		{"empty workdir", func(ctx *domain.RuntimeContext) { ctx.Workdir = "" }, "workdir is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := runtimeContext("m", "s", "claude-cli", "model", "high")
			tt.mutate(&ctx)
			_, err := domain.NewSession("s1", ctx, testNow)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
	_, err := domain.NewSession("", runtimeContext("m", "s", "claude-cli", "model", "high"), testNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestSession_Deactivate(t *testing.T) {
	s := activeSession(t)
	deactivatedAt := testNow.Add(time.Hour)

	err := s.Deactivate(domain.ReasonShutdown, "", deactivatedAt)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionInactive, s.Status)
	assert.Equal(t, domain.ReasonShutdown, s.InactiveReason)
	assert.Empty(t, s.InactiveError)
	require.NotNil(t, s.DeactivatedAt)
	assert.Equal(t, deactivatedAt, *s.DeactivatedAt)
}

func TestSession_Deactivate_WithError(t *testing.T) {
	s := activeSession(t)
	err := s.Deactivate(domain.ReasonCrashed, "process exited with signal 9", testNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, domain.ReasonCrashed, s.InactiveReason)
	assert.Equal(t, "process exited with signal 9", s.InactiveError)
}

func TestSession_Deactivate_AlreadyInactive(t *testing.T) {
	s := activeSession(t)
	require.NoError(t, s.Deactivate(domain.ReasonShutdown, "", testNow.Add(time.Hour)))
	err := s.Deactivate(domain.ReasonCrashed, "", testNow.Add(2*time.Hour))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func TestSession_Deactivate_EmptyReason(t *testing.T) {
	s := activeSession(t)
	err := s.Deactivate("", "", testNow.Add(time.Hour))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inactive reason is required")
}

func TestSession_Reactivate(t *testing.T) {
	s := activeSession(t)
	require.NoError(t, s.Deactivate(domain.ReasonCrashed, "segfault", testNow.Add(time.Hour)))

	reactivatedAt := testNow.Add(2 * time.Hour)
	err := s.Reactivate(reactivatedAt)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionActive, s.Status)
	assert.Empty(t, s.InactiveReason)
	assert.Empty(t, s.InactiveError)
	assert.Nil(t, s.DeactivatedAt)
	assert.Equal(t, reactivatedAt, s.ActivatedAt)
}

func TestSession_Reactivate_AlreadyActive(t *testing.T) {
	s := activeSession(t)
	err := s.Reactivate(testNow.Add(time.Hour))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not inactive")
}

func TestSession_AddUsage(t *testing.T) {
	s := activeSession(t)
	s.AddUsage(100, 50)
	assert.Equal(t, int64(100), s.TokensIn)
	assert.Equal(t, int64(50), s.TokensOut)
	s.AddUsage(200, 75)
	assert.Equal(t, int64(300), s.TokensIn)
	assert.Equal(t, int64(125), s.TokensOut)
}

func TestSession_UpdateConfig(t *testing.T) {
	s := activeSession(t)

	err := s.UpdateConfig("claude-sonnet-4-6", "max")
	require.NoError(t, err)

	assert.Equal(t, "claude-cli", s.Kind)
	assert.Equal(t, "claude-sonnet-4-6", s.Model)
	assert.Equal(t, "max", s.Effort)
}

func TestSession_UpdateConfig_RequiresActiveSession(t *testing.T) {
	s := activeSession(t)
	require.NoError(t, s.Deactivate(domain.ReasonShutdown, "", testNow.Add(time.Hour)))

	err := s.UpdateConfig("claude-sonnet-4-6", "max")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func activeSession(t *testing.T) *domain.Session {
	t.Helper()
	s, err := domain.NewSession("sess-1", runtimeContext("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"), testNow)
	require.NoError(t, err)
	return s
}

func runtimeContext(memberID, spaceID, kind, model, effort string) domain.RuntimeContext {
	return domain.RuntimeContext{
		ProjectID:      "project-1",
		LocationID:     "local",
		MemberID:       memberID,
		SpaceID:        spaceID,
		ChannelID:      "channel:" + spaceID + ":member:" + memberID,
		DisplayName:    "Worker One",
		MemberType:     "worker",
		LifecycleState: "active",
		HarnessKind:    kind,
		Model:          model,
		Effort:         effort,
		Workdir:        "/tmp/project-1",
		SystemPrompt:   "prompt",
		MCPToken:       "token",
		MCPServers:     []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=token"`},
	}
}
