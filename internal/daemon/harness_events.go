package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp"
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

const bootstrapMCPToken = "agen8-local"

func (d *Daemon) restoreActiveMCPSessions(ctx context.Context) error {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	sessions, err := d.app.HarnessSvc.ListActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("list active harness sessions for mcp restore: %w", err)
	}
	for _, session := range sessions {
		if err := d.registerMCPTokenForSession(session); err != nil {
			return fmt.Errorf("restore mcp token for harness session %s: %w", session.ID, err)
		}
		if _, err := d.app.HarnessSvc.RefreshSessionWorkdir(ctx, session.ID); err != nil {
			return fmt.Errorf("refresh workdir for harness session %s: %w", session.ID, err)
		}
		if _, err := d.app.HarnessSvc.RefreshSessionMCPURL(ctx, session.ID, d.mcpURL(session.MCPToken)); err != nil {
			return fmt.Errorf("refresh mcp config for harness session %s: %w", session.ID, err)
		}
	}
	return nil
}

func (d *Daemon) registerHarnessEventHandlers() error {
	if d == nil || d.app == nil {
		return fmt.Errorf("daemon application is required")
	}
	if d.app.EventBus == nil {
		return fmt.Errorf("event bus is required")
	}
	if d.app.HarnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	d.app.EventBus.AddHandler("harness-space-member-lifecycle", eventbus.TopicSpaceMemberLifecycle, d.handleSpaceMemberLifecycleMessage)
	return nil
}

func (d *Daemon) handleSpaceMemberLifecycleMessage(msg *message.Message) error {
	var event eventbus.SpaceMemberLifecycleEvent
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		msg.Nack()
		return fmt.Errorf("unmarshal space member lifecycle event: %w", err)
	}
	if err := d.handleSpaceMemberLifecycle(msg.Context(), event); err != nil {
		msg.Nack()
		return err
	}
	msg.Ack()
	return nil
}

func (d *Daemon) handleSpaceMemberLifecycle(ctx context.Context, event eventbus.SpaceMemberLifecycleEvent) error {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	memberID := strings.TrimSpace(event.MemberID)
	spaceID := strings.TrimSpace(event.SpaceID)
	if memberID == "" {
		return fmt.Errorf("member lifecycle event missing memberId")
	}
	if spaceID == "" {
		return fmt.Errorf("member lifecycle event missing spaceId")
	}

	switch strings.TrimSpace(event.EventType) {
	case eventbus.SpaceMemberEventRegistered:
		_, err := d.app.HarnessSvc.ActivateSession(ctx, harnessActivationFromEvent(event))
		if err != nil {
			return fmt.Errorf("activate harness session for member %s: %w", memberID, err)
		}
		if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(memberID)); err != nil {
			return fmt.Errorf("start message delivery for member %s: %w", memberID, err)
		}
	case eventbus.SpaceMemberEventConfigChanged:
		if err := d.app.HarnessSvc.ValidateRuntimeConfig(event.HarnessKind, event.Model, event.Effort, event.PermissionMode, event.ConfigRef); err != nil {
			return fmt.Errorf("validate harness config for member %s: %w", memberID, err)
		}
		active, err := d.app.HarnessSvc.GetActiveSession(ctx, memberID)
		if err != nil {
			return fmt.Errorf("load active harness session for member %s: %w", memberID, err)
		}
		if active != nil && strings.TrimSpace(active.Kind) == strings.TrimSpace(event.HarnessKind) {
			if _, err := d.app.HarnessSvc.UpdateSessionRuntimeContext(ctx, active.ID, harnessActivationFromEvent(event)); err != nil {
				return fmt.Errorf("update harness session context for member %s config change: %w", memberID, err)
			}
			if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(memberID)); err != nil {
				return fmt.Errorf("start message delivery for member %s: %w", memberID, err)
			}
			return nil
		}
		if active != nil {
			d.app.MessageSvc.StopAgentDelivery(member.ID(memberID))
			if err := d.app.HarnessSvc.DeactivateSession(ctx, active.ID, harnessdomain.ReasonConfigChanged, ""); err != nil {
				return fmt.Errorf("deactivate harness session for member %s config change: %w", memberID, err)
			}
		}
		if _, err := d.app.HarnessSvc.ActivateSession(ctx, harnessActivationFromEvent(event)); err != nil {
			return fmt.Errorf("activate replacement harness session for member %s: %w", memberID, err)
		}
		if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(memberID)); err != nil {
			return fmt.Errorf("start message delivery for member %s: %w", memberID, err)
		}
	case eventbus.SpaceMemberEventIdentityChanged:
		active, err := d.app.HarnessSvc.GetActiveSession(ctx, memberID)
		if err != nil {
			return fmt.Errorf("load active harness session for member %s: %w", memberID, err)
		}
		if active != nil {
			d.app.MessageSvc.StopAgentDelivery(member.ID(memberID))
			if err := d.app.HarnessSvc.DeactivateSession(ctx, active.ID, harnessdomain.ReasonConfigChanged, ""); err != nil {
				return fmt.Errorf("deactivate harness session for member %s identity change: %w", memberID, err)
			}
		}
		if _, err := d.app.HarnessSvc.ActivateSession(ctx, harnessActivationFromEvent(event)); err != nil {
			return fmt.Errorf("activate replacement harness session for member %s identity change: %w", memberID, err)
		}
		if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(memberID)); err != nil {
			return fmt.Errorf("start message delivery for member %s: %w", memberID, err)
		}
	case eventbus.SpaceMemberEventRemoved:
		active, err := d.app.HarnessSvc.GetActiveSession(ctx, memberID)
		if err != nil {
			return fmt.Errorf("load active harness session for member %s: %w", memberID, err)
		}
		if active == nil {
			d.app.MessageSvc.StopAgentDelivery(member.ID(memberID))
			return nil
		}
		d.app.MessageSvc.StopAgentDelivery(member.ID(memberID))
		if err := d.app.HarnessSvc.DeactivateSession(ctx, active.ID, harnessdomain.ReasonMemberRemoved, ""); err != nil {
			return fmt.Errorf("deactivate harness session for removed member %s: %w", memberID, err)
		}
		if d.mcpTokens != nil {
			d.mcpTokens.Revoke("mcp-token-" + memberID)
		}
	default:
		return fmt.Errorf("unsupported space member lifecycle event %q", event.EventType)
	}
	return nil
}

func harnessActivationFromEvent(event eventbus.SpaceMemberLifecycleEvent) harnessapp.ActivateSessionParams {
	return harnessapp.ActivateSessionParams{
		ProjectID:      event.ProjectID,
		MemberID:       event.MemberID,
		SpaceID:        event.SpaceID,
		ChannelID:      event.ChannelID,
		DisplayName:    event.DisplayName,
		MemberType:     event.MemberType,
		LifecycleState: event.LifecycleState,
		HarnessKind:    event.HarnessKind,
		Model:          event.Model,
		Effort:         event.Effort,
		PermissionMode: event.PermissionMode,
		ConfigRef:      event.ConfigRef,
	}
}

func (d *Daemon) ProvisionMCP(_ context.Context, p harnessapp.ActivateSessionParams) (harnessapp.MCPProvisioning, error) {
	if d == nil || d.mcpTokens == nil {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("mcp token store is required")
	}
	if d.app == nil {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("application is required")
	}
	memberID := strings.TrimSpace(p.MemberID)
	spaceID := strings.TrimSpace(p.SpaceID)
	channelID := strings.TrimSpace(p.ChannelID)
	projectID := strings.TrimSpace(p.ProjectID)
	if memberID == "" {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("memberID is required")
	}
	if spaceID == "" {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("spaceID is required")
	}
	if channelID == "" {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("channelID is required")
	}
	if projectID == "" {
		return harnessapp.MCPProvisioning{}, fmt.Errorf("projectID is required")
	}

	token := strings.TrimSpace(p.MCPToken)
	if token == "" {
		token = "mcp-token-" + memberID
	}
	session := &harnessdomain.Session{
		ProjectID: p.ProjectID,
		MemberID:  memberID,
		SpaceID:   spaceID,
		ChannelID: channelID,
		Kind:      strings.TrimSpace(p.HarnessKind),
		MCPToken:  token,
	}
	if err := d.registerMCPTokenForSession(session); err != nil {
		return harnessapp.MCPProvisioning{}, err
	}
	return harnessapp.MCPProvisioning{Token: token, URL: d.mcpURL(token)}, nil
}

func (d *Daemon) registerBootstrapMCPToken() error {
	if d == nil || d.mcpTokens == nil {
		return fmt.Errorf("mcp token store is required")
	}
	d.mcpTokens.Register(bootstrapMCPToken, d.mcpSessionFor(&harnessdomain.Session{
		MCPToken: bootstrapMCPToken,
		Kind:     "codex",
	}))
	return nil
}

func (d *Daemon) registerMCPTokenForSession(session *harnessdomain.Session) error {
	if d == nil || d.mcpTokens == nil {
		return fmt.Errorf("mcp token store is required")
	}
	if d.app == nil {
		return fmt.Errorf("application is required")
	}
	if session == nil {
		return fmt.Errorf("harness session is required")
	}
	token := strings.TrimSpace(session.MCPToken)
	memberID := strings.TrimSpace(session.MemberID)
	spaceID := strings.TrimSpace(session.SpaceID)
	channelID := strings.TrimSpace(session.ChannelID)
	projectID := strings.TrimSpace(session.ProjectID)
	if token == "" {
		return fmt.Errorf("mcp token is required")
	}
	if memberID == "" {
		return fmt.Errorf("memberID is required")
	}
	if spaceID == "" {
		return fmt.Errorf("spaceID is required")
	}
	if channelID == "" {
		return fmt.Errorf("channelID is required")
	}
	if projectID == "" {
		return fmt.Errorf("projectID is required")
	}
	d.mcpTokens.Register(token, d.mcpSessionFor(session))
	return nil
}

func (d *Daemon) mcpSessionFor(session *harnessdomain.Session) mcp.Session {
	if session == nil {
		session = &harnessdomain.Session{}
	}
	channelID := strings.TrimSpace(session.ChannelID)
	spaceID := strings.TrimSpace(session.SpaceID)
	memberID := strings.TrimSpace(session.MemberID)
	projectID := strings.TrimSpace(session.ProjectID)
	return mcp.Session{
		Token:             strings.TrimSpace(session.MCPToken),
		ChannelID:         types.ChannelID(channelID),
		SpaceID:           spacedomain.SpaceID(spaceID),
		MemberID:          memberID,
		HarnessKind:       strings.TrimSpace(session.Kind),
		ContextRegistrar:  d,
		SpaceReader:       d.app.SpaceSvc,
		MemberDirectory:   d.app.SpaceSvc,
		MemberRegistrar:   d.app.SpaceSvc,
		TaskMembers:       d.app.SpaceSvc,
		MessagePublisher:  d.app.MessageSvc,
			DecisionService:   d.app.DecisionSvc,
			GraphService:      d.app.GraphSvc,
			HumanInputAwaiter: d.app.HumanInputMCPAwaiter,
			TaskService:       d.app.TaskSvc,
		ScheduleService:   d.app.ScheduleSvc,
		OperatorService:   d.app.OperatorSvc,
		MissionService:    d.app.MissionSvc,
		MissionKRs:        d.app.MissionSvc,
		MissionProgress:   d.app.MissionSvc,
		ProjectID:         projectID,
	}
}

func (d *Daemon) mcpURL(token string) string {
	values := url.Values{}
	values.Set("token", token)
	return "http://" + strings.TrimSpace(d.cfg.HTTPAddr) + "/mcp?" + values.Encode()
}
