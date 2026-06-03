package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		if session == nil {
			continue
		}
		if strings.TrimSpace(session.MCPToken) != bootstrapMCPToken {
			healed, err := d.refreshBootstrapSessionMCPBinding(ctx, session)
			if err != nil {
				return fmt.Errorf("refresh bootstrap mcp binding for harness session %s: %w", session.ID, err)
			}
			session = healed
		}
		if _, err := d.app.HarnessSvc.RefreshSessionWorkdir(ctx, session.ID); err != nil {
			return fmt.Errorf("refresh workdir for harness session %s: %w", session.ID, err)
		}
		if _, err := d.app.HarnessSvc.RefreshSessionMCPURL(ctx, session.ID, d.mcpURL(session.MCPToken)); err != nil {
			return fmt.Errorf("refresh mcp config for harness session %s: %w", session.ID, err)
		}
	}
	return d.restoreUnambiguousMCPBindings(ctx)
}

func (d *Daemon) refreshBootstrapSessionMCPBinding(ctx context.Context, session *harnessdomain.Session) (*harnessdomain.Session, error) {
	if session == nil {
		return nil, fmt.Errorf("harness session is required")
	}
	return d.app.HarnessSvc.RefreshSessionMCPBinding(ctx, session.ID, bootstrapMCPToken, d.mcpURL(bootstrapMCPToken))
}

func (d *Daemon) restoreUnambiguousMCPBindings(ctx context.Context) error {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	sessions, err := d.app.HarnessSvc.ListActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("list active harness sessions for mcp binding restore: %w", err)
	}
	byToken := make(map[string][]*harnessdomain.Session)
	for _, session := range sessions {
		if session == nil {
			continue
		}
		token := strings.TrimSpace(session.MCPToken)
		if token == "" {
			continue
		}
		byToken[token] = append(byToken[token], session)
	}
	for token, sessions := range byToken {
		if len(sessions) == 1 {
			d.mcpBinding.bind(token, sessions[0].ID)
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
		token = bootstrapMCPToken
	}
	return harnessapp.MCPProvisioning{Token: token, URL: d.mcpURL(token)}, nil
}

func (d *Daemon) registerBootstrapMCPToken() error {
	if d == nil || d.mcpTokens == nil {
		return fmt.Errorf("mcp token store is required")
	}
	session := d.mcpSessionFor(&harnessdomain.Session{
		MCPToken: bootstrapMCPToken,
		Kind:     "codex",
	})
	session.Bootstrap = true
	session.UserID = d.currentMCPUser(context.Background())
	d.mcpTokens.Register(bootstrapMCPToken, session)
	return nil
}

func (d *Daemon) resolveMCPSessionForRequest(ctx context.Context, token string, header http.Header, body []byte) (mcp.Session, error) {
	if d == nil || d.mcpTokens == nil {
		return mcp.Session{}, fmt.Errorf("mcp token store is required")
	}
	base, err := d.mcpTokens.Resolve(token)
	if err != nil {
		return mcp.Session{}, err
	}
	if !base.Bootstrap {
		return base, nil
	}
	base.UserID = d.currentMCPUser(ctx)
	sessionID, threadID := mcp.SessionRefsFromHTTPHeader(header)
	reqCtx := mcp.SessionRequestContext{}
	if len(body) > 0 {
		reqCtx = mcp.SessionRequestContextFromJSONRPCBody(body)
	}
	if d != nil && d.logger != nil {
		d.logger.InfoContext(ctx, "mcp session request context resolved",
			"token", token,
			"header_session_id", sessionID,
			"header_thread_id", threadID,
			"body_session_id", reqCtx.SessionID,
			"body_thread_id", reqCtx.ThreadID,
			"body_turn_id", reqCtx.TurnID,
		)
	}
	if strings.TrimSpace(reqCtx.SessionID) != "" {
		sessionID = reqCtx.SessionID
	}
	if strings.TrimSpace(reqCtx.ThreadID) != "" {
		threadID = reqCtx.ThreadID
	}
	sessionRef := strings.TrimSpace(threadID)
	if sessionRef == "" {
		sessionRef = strings.TrimSpace(sessionID)
	}
	if strings.TrimSpace(reqCtx.TurnID) != "" && sessionRef != "" {
		d.mcpBinding.bindActiveCodexTurn("", sessionRef, reqCtx.TurnID)
		if d != nil && d.logger != nil {
			d.logger.InfoContext(ctx, "mcp active codex turn captured by native ref",
				"thread_id", sessionRef,
				"turn_id", reqCtx.TurnID,
			)
		}
	}
	if sessionRef == "" {
		active, err := d.resolveBoundMCPSession(ctx, token)
		if err != nil {
			return mcp.Session{}, err
		}
		if active == nil {
			return base, nil
		}
		if err := d.registerMCPTokenForSession(active); err != nil {
			return mcp.Session{}, err
		}
		return d.mcpSessionFor(active), nil
	}
	active, err := d.resolveActiveHarnessSessionByRef(ctx, sessionRef)
	if err != nil {
		return mcp.Session{}, err
	}
	if active == nil {
		active, err = d.resolveBoundMCPSession(ctx, token)
		if err != nil {
			return mcp.Session{}, err
		}
		if active == nil {
			return base, nil
		}
	}
	if err := d.registerMCPTokenForSession(active); err != nil {
		return mcp.Session{}, err
	}
	if strings.TrimSpace(reqCtx.TurnID) != "" && strings.TrimSpace(active.Ref) != "" {
		d.mcpBinding.bindActiveCodexTurn(active.ID, active.Ref, reqCtx.TurnID)
		if d != nil && d.logger != nil {
			d.logger.InfoContext(ctx, "mcp active codex turn bound",
				"session_id", active.ID,
				"thread_id", active.Ref,
				"turn_id", reqCtx.TurnID,
				"member_id", active.MemberID,
			)
		}
	} else if d != nil && d.logger != nil {
		d.logger.InfoContext(ctx, "mcp active codex turn not bound",
			"session_id", active.ID,
			"thread_id", active.Ref,
			"turn_id", reqCtx.TurnID,
			"member_id", active.MemberID,
		)
	}
	return d.mcpSessionFor(active), nil
}

func (d *Daemon) resolveBoundMCPSession(ctx context.Context, token string) (*harnessdomain.Session, error) {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return nil, fmt.Errorf("harness service is required")
	}
	sessionID := d.mcpBinding.sessionID(token)
	if sessionID == "" {
		return nil, nil
	}
	active, err := d.app.HarnessSvc.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load bound mcp harness session %s: %w", sessionID, err)
	}
	if active == nil || active.Status != harnessdomain.SessionActive || strings.TrimSpace(active.MCPToken) != strings.TrimSpace(token) {
		d.mcpBinding.unbind(token)
		return nil, nil
	}
	return active, nil
}

func (d *Daemon) resolveActiveHarnessSessionByRef(ctx context.Context, sessionRef string) (*harnessdomain.Session, error) {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return nil, fmt.Errorf("harness service is required")
	}
	sessionRef = strings.TrimSpace(sessionRef)
	if sessionRef == "" {
		return nil, nil
	}
	sessions, err := d.app.HarnessSvc.ListActiveSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active harness sessions for mcp session ref: %w", err)
	}
	var matches []*harnessdomain.Session
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if strings.TrimSpace(session.Ref) == sessionRef {
			matches = append(matches, session)
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}
	for _, session := range matches {
		if strings.EqualFold(strings.TrimSpace(session.Kind), "codex") {
			return session, nil
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return nil, fmt.Errorf("multiple active harness sessions use native session ref %q", sessionRef)
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
		UserID:            d.currentMCPUser(context.Background()),
		ChannelID:         types.ChannelID(channelID),
		SpaceID:           spacedomain.SpaceID(spaceID),
		MemberID:          memberID,
		HarnessKind:       strings.TrimSpace(session.Kind),
		ContextRegistrar:  d,
		SpaceSetup:        d.app.SpaceSvc,
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
