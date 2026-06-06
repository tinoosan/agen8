package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/logging"
	authapp "github.com/tinoosan/agen8-mcp-server/internal/services/auth/app"
	authdomain "github.com/tinoosan/agen8-mcp-server/internal/services/auth/domain"
	authinfra "github.com/tinoosan/agen8-mcp-server/internal/services/auth/infra"
	credentialapp "github.com/tinoosan/agen8-mcp-server/internal/services/credential/app"
	credentialinfra "github.com/tinoosan/agen8-mcp-server/internal/services/credential/infra"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	decisioninfra "github.com/tinoosan/agen8-mcp-server/internal/services/decision/infra"
	fileapp "github.com/tinoosan/agen8-mcp-server/internal/services/file/app"
	fileinfra "github.com/tinoosan/agen8-mcp-server/internal/services/file/infra"
	graphapp "github.com/tinoosan/agen8-mcp-server/internal/services/graph/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/contextlink"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
	locationinfra "github.com/tinoosan/agen8-mcp-server/internal/services/location/infra"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	missioninfra "github.com/tinoosan/agen8-mcp-server/internal/services/mission/infra"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	taskinfra "github.com/tinoosan/agen8-mcp-server/internal/services/task/infra"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	userinfra "github.com/tinoosan/agen8-mcp-server/internal/services/user/infra"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
)

// Application is the process-scoped composition root for the stripped
// MCP-first work-context server.
type Application struct {
	AuthSvc       *authapp.Service
	UserSvc       *userapp.Service
	CredentialSvc *credentialapp.Service
	GraphSvc      *graphapp.Service
	GraphLinks    contextlink.Repository
	TaskSvc       *taskapp.Service
	MissionSvc    *missionapp.Service
	ProjectSvc    *projectapp.Service
	FileSvc       *fileapp.Service
	LocationSvc   *locationapp.Service
	EventBus      *eventbus.Bus
	DecisionSvc   *decisionapp.Service
}

// NewApplication builds the retained service graph.
func NewApplication(cfg Config) (*Application, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	handle, err := implstore.GetDBHandle(context.Background(), cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("open db handle: %w", err)
	}

	userRepo, err := userinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build user repository: %w", err)
	}
	userSvc, err := userapp.NewService(userRepo, userdomain.SystemClock{}, logger.With("service", "user"))
	if err != nil {
		return nil, fmt.Errorf("build user service: %w", err)
	}

	authRepos, err := authinfra.NewRepositories(handle)
	if err != nil {
		return nil, fmt.Errorf("build auth repositories: %w", err)
	}
	authSvc, err := authapp.NewService(
		authRepos.Passwords,
		authRepos.Sessions,
		authRepos.APIKeys,
		authRepos.LinkTokens,
		userSvc,
		authdomain.SystemClock{},
		logger.With("service", "auth"),
	)
	if err != nil {
		return nil, fmt.Errorf("build auth service: %w", err)
	}

	credentialRepo, err := credentialinfra.NewRepository(handle, cfg.Host.DataDir)
	if err != nil {
		return nil, fmt.Errorf("build credential repository: %w", err)
	}
	credentialSvc, err := credentialapp.NewService(credentialapp.Config{Repository: credentialRepo})
	if err != nil {
		return nil, fmt.Errorf("build credential service: %w", err)
	}

	contextLinkRepo, err := contextlink.NewSQLiteRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build context link repository: %w", err)
	}
	bus := eventbus.New(nil)

	projectRepo, err := projectinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build project repository: %w", err)
	}
	memberRepo, err := projectinfra.NewMemberRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build project member repository: %w", err)
	}
	workspaceRepo, err := projectinfra.NewWorkspaceRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build project workspace repository: %w", err)
	}
	projectSvc, err := projectapp.NewService(projectapp.Config{
		Projects:   projectRepo,
		Members:    memberRepo,
		Workspaces: workspaceRepo,
		LinkTokens: linkTokenIssuerAdapter{auth: authSvc},
		Caller:     caller.ContextResolver{},
		Configs:    permissiveRuntimeConfigValidator{},
		Events:     bus,
		Logger:     logger.With("service", "project"),
	})
	if err != nil {
		return nil, fmt.Errorf("build project service: %w", err)
	}

	// Retire the duplicate member rows left by the old harness-label fork bug. Resolve
	// already heals such forks on read, so this is a data-hygiene pass, not a correctness
	// gate: a failure is logged loudly but must not stop the daemon from starting. It is a
	// no-op once each session has a single active member, so it is safe to run every boot.
	if retired, err := projectSvc.ReconcileDuplicateMembers(context.Background()); err != nil {
		logger.Error("reconcile duplicate session-fork members at startup", "error", err)
	} else if retired > 0 {
		logger.Info("reconciled duplicate session-fork members at startup", "retired", retired)
	}

	locationRepo, err := locationinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build location repository: %w", err)
	}
	locationSvc, err := locationapp.NewService(locationapp.Config{
		Locations: locationRepo,
		Transport: locationinfra.NewTransport(locationinfra.TransportConfig{
			Credentials:     credentialSvc,
			LocalDaemonAddr: cfg.DaemonHTTPAddr,
			Logger:          logger.With("service", "location.transport"),
		}),
		Projects: projectLocationChecker{projects: projectSvc},
		Logger:   logger.With("service", "location"),
	})
	if err != nil {
		return nil, fmt.Errorf("build location service: %w", err)
	}
	if _, err := locationSvc.EnsureLocal(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure local location: %w", err)
	}

	fileRepo, err := fileinfra.NewLocationRepository(
		locationRepo,
		locationinfra.NewTransport(locationinfra.TransportConfig{
			Credentials:     credentialSvc,
			LocalDaemonAddr: cfg.DaemonHTTPAddr,
			Logger:          logger.With("service", "file.location.transport"),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build file repository: %w", err)
	}
	fileSvc, err := fileapp.NewService(fileapp.Config{Files: fileRepo, Projects: projectSvc})
	if err != nil {
		return nil, fmt.Errorf("build file service: %w", err)
	}

	taskRepo, err := taskinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build task repository: %w", err)
	}
	taskSvc, err := taskapp.NewService(
		taskRepo,
		taskdomain.SystemClock{},
		caller.ContextResolver{},
		projectSvc,
		projectLoaderAdapter{projects: projectSvc},
		logger.With("service", "task"),
	)
	if err != nil {
		return nil, fmt.Errorf("build task service: %w", err)
	}

	missionRepos, err := missioninfra.NewRepositories(handle)
	if err != nil {
		return nil, fmt.Errorf("build mission repositories: %w", err)
	}
	missionSvc, err := missionapp.NewService(
		missionRepos.Missions,
		missionRepos.KeyResults,
		missionRepos.ProgressEntries,
		missionRepos.LifecycleEvents,
		missiondomain.SystemClock{},
		caller.ContextResolver{},
		projectLoaderAdapter{projects: projectSvc},
		taskSvc,
		missionLinkedTaskLoader{},
		missionEventPublisher{bus: bus},
		logger.With("service", "mission"),
	)
	if err != nil {
		return nil, fmt.Errorf("build mission service: %w", err)
	}

	graphLinks := &graphServiceLinkPort{}
	taskSvc.SetGraphLinkWriter(graphLinks)
	taskSvc.SetKeyResultMissionReader(missionRefResolver{missions: missionSvc})

	decisionRepo := decisioninfra.NewSQLiteRepository(handle)
	decisionSvc, err := decisionapp.NewService(
		decisionRepo,
		decisiondomain.SystemClock{},
		graphLinks,
		graphLinks,
		bus,
		nil,
		nil,
		projectSvc,
		logger.With("service", "decision"),
	)
	if err != nil {
		return nil, fmt.Errorf("build decision service: %w", err)
	}

	graphSvc, err := graphapp.NewService(
		contextLinkRepo,
		graphapp.DefaultHydrators(taskSvc, decisionSvc, missionSvc),
		2*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("build graph service: %w", err)
	}
	graphLinks.svc = graphSvc

	return &Application{
		AuthSvc:       authSvc,
		UserSvc:       userSvc,
		CredentialSvc: credentialSvc,
		GraphSvc:      graphSvc,
		GraphLinks:    contextLinkRepo,
		TaskSvc:       taskSvc,
		MissionSvc:    missionSvc,
		ProjectSvc:    projectSvc,
		FileSvc:       fileSvc,
		LocationSvc:   locationSvc,
		EventBus:      bus,
		DecisionSvc:   decisionSvc,
	}, nil
}

type permissiveRuntimeConfigValidator struct{}

func (permissiveRuntimeConfigValidator) ValidateConfig(harnessKind, model, effort string) error {
	if strings.TrimSpace(harnessKind) == "" {
		return fmt.Errorf("harnessKind is required")
	}
	return nil
}

func (permissiveRuntimeConfigValidator) ValidateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef string) error {
	return permissiveRuntimeConfigValidator{}.ValidateConfig(harnessKind, model, effort)
}

func (permissiveRuntimeConfigValidator) DefaultPermissionMode(harnessKind string) string {
	return strings.TrimSpace(harnessKind) + "/default"
}

func (permissiveRuntimeConfigValidator) CompatibilityPermissionMode(harnessKind string) string {
	return strings.TrimSpace(harnessKind) + "/default"
}

type projectLoaderAdapter struct {
	projects *projectapp.Service
}

func (a projectLoaderAdapter) Get(ctx context.Context, projectID types.ProjectID) (projectdomain.Project, error) {
	if a.projects == nil {
		return projectdomain.Project{}, fmt.Errorf("project service is required")
	}
	return a.projects.GetProject(ctx, projectID)
}

// linkTokenIssuerAdapter lets the project service mint wlt_ link tokens without
// importing the auth packages. The project service performs the ownership check;
// this adapter (which the composition root is uniquely allowed to write, knowing
// both services) translates the opaque strings of projectapp.LinkTokenRequest
// into auth's typed ids and forwards to auth.CreateLinkToken.
type linkTokenIssuerAdapter struct {
	auth *authapp.Service
}

func (a linkTokenIssuerAdapter) IssueLinkToken(ctx context.Context, req projectapp.LinkTokenRequest) (projectapp.LinkTokenIssued, error) {
	if a.auth == nil {
		return projectapp.LinkTokenIssued{}, fmt.Errorf("auth service is required")
	}
	userID, err := userdomain.NewID(req.UserID)
	if err != nil {
		return projectapp.LinkTokenIssued{}, fmt.Errorf("link token user id: %w", err)
	}
	result, err := a.auth.CreateLinkToken(ctx, authapp.CreateLinkTokenParams{
		UserID:      userID,
		ProjectID:   req.ProjectID,
		WorkspaceID: req.WorkspaceID,
		Label:       req.Label,
		ExpiresAt:   req.ExpiresAt,
	})
	if err != nil {
		return projectapp.LinkTokenIssued{}, err
	}
	lt := result.LinkToken
	return projectapp.LinkTokenIssued{
		ID:          lt.ID.String(),
		Prefix:      lt.Prefix,
		Token:       result.Token,
		UserID:      lt.UserID.String(),
		ProjectID:   lt.ProjectID,
		WorkspaceID: lt.WorkspaceID,
		Label:       lt.Label,
		ExpiresAt:   lt.ExpiresAt,
		CreatedAt:   lt.CreatedAt,
	}, nil
}

type graphServiceLinkPort struct {
	svc *graphapp.Service
}

func (p *graphServiceLinkPort) Link(ctx context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error) {
	if p == nil || p.svc == nil {
		return graphdomain.GraphEdge{}, nil, fmt.Errorf("graph service is required")
	}
	return p.svc.Link(ctx, req)
}

func (p *graphServiceLinkPort) DeleteLinksForNode(ctx context.Context, nodeType, nodeID string) error {
	if p == nil || p.svc == nil {
		return fmt.Errorf("graph service is required")
	}
	return p.svc.DeleteLinksForNode(ctx, nodeType, nodeID)
}

type missionRefResolver struct {
	missions *missionapp.Service
}

func (r missionRefResolver) KeyResultMission(ctx context.Context, krRef string) (string, error) {
	if r.missions == nil {
		return "", fmt.Errorf("mission service is required")
	}
	krRef = strings.TrimSpace(krRef)
	if krRef == "" {
		return "", fmt.Errorf("key result ref is required")
	}
	keyResult, err := r.missions.GetKeyResult(ctx, krdomain.KeyResultID(krRef))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(keyResult.MissionID)), nil
}

type projectLocationChecker struct {
	projects *projectapp.Service
}

func (c projectLocationChecker) HasProjectsForLocation(ctx context.Context, locationID locationdomain.ID) (bool, error) {
	if c.projects == nil {
		return false, fmt.Errorf("project service is required")
	}
	projects, err := c.projects.ListProjects(ctx, projectdomain.Filter{})
	if err != nil {
		return false, err
	}
	for _, project := range projects {
		if string(project.LocationID()) == string(locationID) {
			return true, nil
		}
	}
	return false, nil
}

type missionLinkedTaskLoader struct{}

func (missionLinkedTaskLoader) ListTaskIDsForKeyResult(context.Context, krdomain.KeyResultID) ([]taskdomain.TaskID, error) {
	return nil, nil
}

type missionEventPublisher struct {
	bus *eventbus.Bus
}

func (p missionEventPublisher) Append(_ context.Context, event types.EventRecord) error {
	if p.bus == nil {
		return fmt.Errorf("mission event bus is required")
	}
	switch event.Type {
	case string(missionapp.MissionEventActivated),
		string(missionapp.MissionEventPaused),
		string(missionapp.MissionEventCompleted),
		string(missionapp.MissionEventArchived):
		return p.bus.Publish(eventbus.TopicMissionLifecycle, event)
	default:
		return p.bus.Publish(eventbus.TopicKRProgress, event)
	}
}

var _ slog.Handler
