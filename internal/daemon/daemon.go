package daemon

import (
	"context"
	"fmt"
	"sync"

	"github.com/tinoosan/agen8-mcp-server/internal/app"
	"github.com/tinoosan/agen8-mcp-server/internal/mcp"
	"github.com/tinoosan/agen8-mcp-server/internal/rpc"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
	"github.com/tinoosan/agen8-mcp-server/pkg/signalhub"
)

type Daemon struct {
	cfg       Config
	app       *app.Application
	rpc       *rpc.Server
	mcpTokens *mcp.TokenStore
	mcp       *mcp.Server
	events    *signalhub.PayloadHub[string, protocol.Message]
	identity  localIdentityTracker
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
	d := &Daemon{
		cfg:       cfg,
		app:       application,
		rpc:       server,
		mcpTokens: mcpTokens,
		mcp:       mcpServer,
		events:    signalhub.NewPayload[string, protocol.Message](),
	}
	application.HarnessSvc.SetMCPProvisioner(d)
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
	if err := d.startMessageDeliveryForActiveSessions(ctx); err != nil {
		return err
	}
	if err := d.app.HarnessSvc.StartExternalSessionSyncForActiveSessions(ctx); err != nil {
		return err
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
