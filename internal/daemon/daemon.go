package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/tinoosan/agen8-mcp-server/internal/app"
	"github.com/tinoosan/agen8-mcp-server/internal/logging"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp"
	"github.com/tinoosan/agen8-mcp-server/internal/rpc"
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	codexruntime "github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra/codex"
	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/signalhub"
)

type Daemon struct {
	cfg        Config
	app        *app.Application
	rpc        *rpc.Server
	mcpTokens  *mcp.TokenStore
	mcp        *mcp.Server
	events     *signalhub.PayloadHub[string, protocol.Message]
	identity   localIdentityTracker
	mcpBinding mcpSessionBindingTracker
	logger     *slog.Logger
}

type localIdentityTracker struct {
	mu     sync.RWMutex
	userID string
}

func New(cfg Config) (*Daemon, error) {
	cfg, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	application, err := app.NewApplication(app.Config{
		Host:           cfg.AppConfig,
		Logging:        cfg.Logging,
		DaemonHTTPAddr: cfg.HTTPAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("build application: %w", err)
	}
	if application.AuthSvc == nil {
		return nil, fmt.Errorf("auth service is required")
	}
	if application.UserSvc == nil {
		return nil, fmt.Errorf("user service is required")
	}
	if application.TaskSvc == nil {
		return nil, fmt.Errorf("task service is required")
	}
	if application.ScheduleSvc == nil {
		return nil, fmt.Errorf("schedule service is required")
	}
	if application.DecisionSvc == nil {
		return nil, fmt.Errorf("decision service is required")
	}
	if application.MissionSvc == nil {
		return nil, fmt.Errorf("mission service is required")
	}
	if application.SpaceSvc == nil {
		return nil, fmt.Errorf("space service is required")
	}
	if application.CredentialSvc == nil {
		return nil, fmt.Errorf("credential service is required")
	}
	if application.MessageSvc == nil {
		return nil, fmt.Errorf("message service is required")
	}
	if application.ProjectSvc == nil {
		return nil, fmt.Errorf("project service is required")
	}
	if application.FileSvc == nil {
		return nil, fmt.Errorf("file service is required")
	}
	if application.LocationSvc == nil {
		return nil, fmt.Errorf("location service is required")
	}
	if application.HarnessSvc == nil {
		return nil, fmt.Errorf("harness service is required")
	}
	if application.OperatorSvc == nil {
		return nil, fmt.Errorf("operator service is required")
	}
	if application.HumanInputSvc == nil || application.HumanInputWake == nil {
		return nil, fmt.Errorf("human input service is required")
	}
	reg := rpc.NewRegistry()
	if err := rpc.RegisterAuth(reg, application.AuthSvc); err != nil {
		return nil, fmt.Errorf("register auth rpc: %w", err)
	}
	if err := rpc.RegisterUser(reg, application.UserSvc); err != nil {
		return nil, fmt.Errorf("register user rpc: %w", err)
	}
	if err := rpc.RegisterSpace(reg, application.SpaceSvc); err != nil {
		return nil, fmt.Errorf("register space rpc: %w", err)
	}
	if err := rpc.RegisterCredential(reg, application.CredentialSvc); err != nil {
		return nil, fmt.Errorf("register credential rpc: %w", err)
	}
	if err := rpc.RegisterTask(reg, application.TaskSvc); err != nil {
		return nil, fmt.Errorf("register task rpc: %w", err)
	}
	if err := rpc.RegisterSchedule(reg, application.ScheduleSvc); err != nil {
		return nil, fmt.Errorf("register schedule rpc: %w", err)
	}
	if err := rpc.RegisterDecision(reg, application.DecisionSvc, application.SpaceSvc, application.UserSvc); err != nil {
		return nil, fmt.Errorf("register decision rpc: %w", err)
	}
	if err := rpc.RegisterGraph(reg, application.GraphSvc, application.GraphLinks); err != nil {
		return nil, fmt.Errorf("register graph rpc: %w", err)
	}
	if err := rpc.RegisterHumanInput(reg, application.HumanInputSvc, application.HumanInputWake); err != nil {
		return nil, fmt.Errorf("register human input rpc: %w", err)
	}
	if err := rpc.RegisterOperator(reg, application.OperatorSvc); err != nil {
		return nil, fmt.Errorf("register operator rpc: %w", err)
	}
	if err := rpc.RegisterMission(reg, application.MissionSvc); err != nil {
		return nil, fmt.Errorf("register mission rpc: %w", err)
	}
	if err := rpc.RegisterMessage(reg, application.MessageSvc, application.SpaceSvc); err != nil {
		return nil, fmt.Errorf("register message rpc: %w", err)
	}
	if err := rpc.RegisterProject(reg, application.ProjectSvc); err != nil {
		return nil, fmt.Errorf("register project rpc: %w", err)
	}
	if err := rpc.RegisterFile(reg, application.FileSvc); err != nil {
		return nil, fmt.Errorf("register file rpc: %w", err)
	}
	if err := rpc.RegisterLocation(reg, application.LocationSvc); err != nil {
		return nil, fmt.Errorf("register location rpc: %w", err)
	}
	if err := rpc.RegisterHarness(reg, application.HarnessSvc); err != nil {
		return nil, fmt.Errorf("register harness rpc: %w", err)
	}
	server, err := rpc.NewServer(reg)
	if err != nil {
		return nil, err
	}
	mcpTokens := mcp.NewTokenStore()
	mcpServer, err := mcp.NewServer(mcpTokens)
	if err != nil {
		return nil, fmt.Errorf("build mcp server: %w", err)
	}
	daemonLogger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("build daemon logger: %w", err)
	}
	d := &Daemon{
		cfg:       cfg,
		app:       application,
		rpc:       server,
		mcpTokens: mcpTokens,
		mcp:       mcpServer,
		events:    signalhub.NewPayload[string, protocol.Message](),
		mcpBinding: mcpSessionBindingTracker{
			byToken:            make(map[string]string),
			appServerBySession: make(map[string]string),
		},
		logger: daemonLogger.With("service", "daemon"),
	}
	mcpServer.SetSessionResolver(d.resolveMCPSessionForRequest)
	application.HarnessSvc.SetMCPProvisioner(d)
	application.HarnessSvc.SetRuntimeHostResolver(d)
	application.MessageSvc.SetHarnessChatSender(d)
	application.MessageSvc.SetConversationNotifier(d)
	application.HumanInputSvc.SetNotifier(d)
	if err := d.registerBootstrapMCPToken(); err != nil {
		return nil, fmt.Errorf("register bootstrap mcp token: %w", err)
	}
	if err := d.restoreActiveMCPSessions(context.Background()); err != nil {
		return nil, fmt.Errorf("restore active mcp sessions: %w", err)
	}
	if err := d.registerHarnessEventHandlers(); err != nil {
		return nil, fmt.Errorf("register harness event handlers: %w", err)
	}
	return d, nil
}

type mcpSessionBindingTracker struct {
	mu                  sync.RWMutex
	byToken             map[string]string
	appServerBySession  map[string]string
	activeTurnBySession map[string]activeCodexTurn
	activeTurnByRef     map[string]activeCodexTurn
}

type activeCodexTurn struct {
	threadID string
	turnID   string
}

func (t *mcpSessionBindingTracker) bind(token string, sessionID string) {
	token = strings.TrimSpace(token)
	sessionID = strings.TrimSpace(sessionID)
	if token == "" || sessionID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byToken == nil {
		t.byToken = make(map[string]string)
	}
	t.byToken[token] = sessionID
}

func (t *mcpSessionBindingTracker) sessionID(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return strings.TrimSpace(t.byToken[token])
}

func (t *mcpSessionBindingTracker) unbind(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byToken, token)
}

func (t *mcpSessionBindingTracker) bindAppServerURL(sessionID string, appServerURL string) {
	sessionID = strings.TrimSpace(sessionID)
	appServerURL = strings.TrimSpace(appServerURL)
	if sessionID == "" || appServerURL == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.appServerBySession == nil {
		t.appServerBySession = make(map[string]string)
	}
	t.appServerBySession[sessionID] = appServerURL
}

func (t *mcpSessionBindingTracker) appServerURL(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return strings.TrimSpace(t.appServerBySession[sessionID])
}

func (t *mcpSessionBindingTracker) bindActiveCodexTurn(sessionID string, threadID string, turnID string) {
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	turn := activeCodexTurn{threadID: threadID, turnID: turnID}
	if sessionID != "" {
		if t.activeTurnBySession == nil {
			t.activeTurnBySession = make(map[string]activeCodexTurn)
		}
		t.activeTurnBySession[sessionID] = turn
	}
	if t.activeTurnByRef == nil {
		t.activeTurnByRef = make(map[string]activeCodexTurn)
	}
	t.activeTurnByRef[threadID] = turn
}

func (t *mcpSessionBindingTracker) activeCodexTurn(sessionID string) activeCodexTurn {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return activeCodexTurn{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.activeTurnBySession[sessionID]
}

func (t *mcpSessionBindingTracker) activeCodexTurnForRef(threadID string) activeCodexTurn {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return activeCodexTurn{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.activeTurnByRef[threadID]
}

var _ harnessapp.RuntimeHostResolver = (*Daemon)(nil)
var _ messageapp.HarnessChatSender = (*Daemon)(nil)

func (d *Daemon) SendMessage(ctx context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	if input.AllowSteering {
		result, err := d.trySteerActiveCodexTurn(ctx, input)
		if err == nil {
			if d != nil && d.logger != nil {
				d.logger.InfoContext(ctx, "agent message steered into active codex turn",
					"member_id", input.MemberID,
					"space_id", input.SpaceID,
					"channel_id", input.ChannelID,
					"conversation_message_id", input.ConversationMessageID,
					"session_id", result.SessionID,
					"turn_id", result.TurnID,
				)
			}
			return result, nil
		}
		if d != nil && d.logger != nil {
			d.logger.InfoContext(ctx, "agent message active-turn steering unavailable",
				"member_id", input.MemberID,
				"space_id", input.SpaceID,
				"channel_id", input.ChannelID,
				"conversation_message_id", input.ConversationMessageID,
				"steer_only", input.SteerOnly,
				"error", err,
			)
		}
		if input.SteerOnly {
			return messageapp.HarnessChatResult{}, err
		}
	}
	return d.sendMessageThroughHarness(ctx, input)
}

func (d *Daemon) trySteerActiveCodexTurn(ctx context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("harness service is required")
	}
	session, err := d.app.HarnessSvc.GetActiveSession(ctx, strings.TrimSpace(input.MemberID))
	if err != nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("load active harness session for member %s: %w", input.MemberID, err)
	}
	if session == nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("member %q has no active harness session", input.MemberID)
	}
	if !strings.EqualFold(strings.TrimSpace(session.Kind), "codex") {
		return messageapp.HarnessChatResult{}, fmt.Errorf("active harness session %q is not codex", session.ID)
	}
	turn := d.mcpBinding.activeCodexTurn(session.ID)
	if strings.TrimSpace(turn.threadID) == "" || strings.TrimSpace(turn.turnID) == "" {
		return messageapp.HarnessChatResult{}, fmt.Errorf("active codex turn is not registered for harness session %q", session.ID)
	}
	host, err := d.ResolveRuntimeHost(ctx, harnessapp.RuntimeHostRequest{
		LocationID:  session.LocationID,
		HarnessKind: session.Kind,
		SessionID:   session.ID,
		ProjectID:   session.ProjectID,
		MemberID:    session.MemberID,
		SessionRef:  session.Ref,
		MCPToken:    session.MCPToken,
	})
	if err != nil {
		return messageapp.HarnessChatResult{}, err
	}
	appServerURL := strings.TrimSpace(host.AppServerURL)
	if appServerURL == "" {
		return messageapp.HarnessChatResult{}, fmt.Errorf("codex app-server url is not registered for harness session %q", session.ID)
	}
	if d.logger != nil {
		d.logger.InfoContext(ctx, "steering agent message to active codex turn",
			"member_id", input.MemberID,
			"session_id", session.ID,
			"thread_id", turn.threadID,
			"turn_id", turn.turnID,
			"app_server_url", appServerURL,
			"conversation_message_id", input.ConversationMessageID,
		)
	}
	params := harnessdomain.StartParams{
		Workdir:         strings.TrimSpace(session.Workdir),
		Model:           strings.TrimSpace(session.Model),
		ReasoningEffort: strings.TrimSpace(session.Effort),
		SystemPrompt:    strings.TrimSpace(session.SystemPrompt),
		MCPServers:      append([]string(nil), session.MCPServers...),
		PermissionMode:  strings.TrimSpace(session.PermissionMode),
		ConfigRef:       strings.TrimSpace(session.ConfigRef),
		SessionRef:      strings.TrimSpace(turn.threadID),
		AppServerURL:    appServerURL,
	}
	if err := codexruntime.SteerAppServerTurn(ctx, params, turn.turnID, input.Text, daemonDomainAttachments(input.Attachments)); err != nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("steer active codex turn: %w", err)
	}
	return messageapp.HarnessChatResult{
		SessionID: session.ID,
		TurnID:    strings.TrimSpace(turn.turnID),
		Delivery:  "steered",
	}, nil
}

func (d *Daemon) sendMessageThroughHarness(ctx context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("harness service is required")
	}
	result, err := d.app.HarnessSvc.SendMessage(ctx, harnessapp.SendMessageParams{
		SpaceID:               input.SpaceID,
		MemberID:              input.MemberID,
		ChannelID:             input.ChannelID,
		ConversationMessageID: input.ConversationMessageID,
		SenderType:            input.SenderType,
		SenderID:              input.SenderID,
		Text:                  input.Text,
		Attachments:           daemonHarnessAttachments(input.Attachments),
		AllowSteering:         input.AllowSteering,
		SteerOnly:             input.SteerOnly,
		OnAssistantDelta: func(ctx context.Context, delta harnessapp.AssistantDelta) error {
			if input.Stream == nil {
				return nil
			}
			return input.Stream.AppendAssistantDelta(ctx, messageapp.HarnessAssistantDelta{
				SessionID: delta.SessionID,
				TurnID:    delta.TurnID,
				Sequence:  delta.Sequence,
				Text:      delta.Text,
			})
		},
		OnThinkingDelta: func(ctx context.Context, delta harnessapp.ThinkingDelta) error {
			if input.Stream == nil {
				return nil
			}
			return input.Stream.AppendThinkingDelta(ctx, messageapp.HarnessThinkingDelta{
				SessionID: delta.SessionID,
				TurnID:    delta.TurnID,
				Sequence:  delta.Sequence,
				Text:      delta.Text,
				Data:      delta.Data,
			})
		},
		OnActivity: func(ctx context.Context, activity harnessapp.ActivityEvent) error {
			if input.Stream == nil {
				return nil
			}
			return input.Stream.AppendActivity(ctx, messageapp.HarnessActivity{
				SessionID:  activity.SessionID,
				TurnID:     activity.TurnID,
				ToolCallID: activity.ToolCallID,
				ToolName:   activity.ToolName,
				Sequence:   activity.Sequence,
				Status:     activity.Status,
				Text:       activity.Text,
				Data:       activity.Data,
			})
		},
	})
	if err != nil {
		return messageapp.HarnessChatResult{}, err
	}
	return messageapp.HarnessChatResult{
		SessionID: result.SessionID,
		RunID:     result.RunID,
		TurnID:    result.TurnID,
		Delivery:  result.Delivery,
		Text:      result.Text,
	}, nil
}

func daemonHarnessAttachments(in []conversation.Attachment) []harnessapp.PromptAttachment {
	out := make([]harnessapp.PromptAttachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, harnessapp.PromptAttachment{
			ID:        attachment.ID,
			Name:      attachment.Name,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
			URI:       attachment.URI,
		})
	}
	return out
}

func daemonDomainAttachments(in []conversation.Attachment) []harnessdomain.PromptAttachment {
	out := make([]harnessdomain.PromptAttachment, 0, len(in))
	for _, attachment := range in {
		out = append(out, harnessdomain.PromptAttachment{
			ID:        attachment.ID,
			Name:      attachment.Name,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
			URI:       attachment.URI,
		})
	}
	return out
}

func (d *Daemon) Run(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("daemon is nil")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 1)
	if d.app != nil && d.app.EventBus != nil {
		go func() {
			if err := d.app.EventBus.Run(ctx); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("run event bus: %w", err)
			}
		}()
		select {
		case <-d.app.EventBus.Running():
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := d.startScheduleRunner(ctx); err != nil {
		return err
	}
	var runErr error
	switch d.cfg.Listener {
	case ListenerLocal:
		runErr = LocalStrategy{}.Run(ctx, d)
	case ListenerHTTP:
		runErr = HTTPStrategy{}.Run(ctx, d)
	default:
		runErr = fmt.Errorf("unknown daemon listener %q", d.cfg.Listener)
	}
	select {
	case err := <-errCh:
		return err
	default:
	}
	return runErr
}

func (d *Daemon) startMessageDeliveryForActiveSessions(ctx context.Context) error {
	if d == nil || d.app == nil || d.app.HarnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	sessions, err := d.app.HarnessSvc.ListActiveSessions(ctx)
	if err != nil {
		return fmt.Errorf("list active harness sessions for message delivery: %w", err)
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if err := d.app.MessageSvc.StartAgentDelivery(ctx, member.ID(session.MemberID)); err != nil {
			return fmt.Errorf("start message delivery for member %s: %w", session.MemberID, err)
		}
	}
	return nil
}

func (d *Daemon) Config() Config {
	if d == nil {
		return Config{}
	}
	return d.cfg
}
