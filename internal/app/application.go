package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/logging"
	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	authdomain "github.com/tinoosan/agen8/internal/services/auth/domain"
	authinfra "github.com/tinoosan/agen8/internal/services/auth/infra"
	"github.com/tinoosan/agen8/internal/services/auth/linktoken"
	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialinfra "github.com/tinoosan/agen8/internal/services/credential/infra"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
	decisioninfra "github.com/tinoosan/agen8/internal/services/decision/infra"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	fileinfra "github.com/tinoosan/agen8/internal/services/file/infra"
	graphapp "github.com/tinoosan/agen8/internal/services/graph/app"
	"github.com/tinoosan/agen8/internal/services/graph/contextlink"
	graphdomain "github.com/tinoosan/agen8/internal/services/graph/domain"
	"github.com/tinoosan/agen8/internal/services/lastseen"
	locationapp "github.com/tinoosan/agen8/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
	locationinfra "github.com/tinoosan/agen8/internal/services/location/infra"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	missioninfra "github.com/tinoosan/agen8/internal/services/mission/infra"
	notificationapp "github.com/tinoosan/agen8/internal/services/notification/app"
	notificationdomain "github.com/tinoosan/agen8/internal/services/notification/domain"
	notificationinfra "github.com/tinoosan/agen8/internal/services/notification/infra"
	pinapp "github.com/tinoosan/agen8/internal/services/pin/app"
	pininfra "github.com/tinoosan/agen8/internal/services/pin/infra"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
	projectinfra "github.com/tinoosan/agen8/internal/services/project/infra"
	questionapp "github.com/tinoosan/agen8/internal/services/question/app"
	questiondomain "github.com/tinoosan/agen8/internal/services/question/domain"
	questioninfra "github.com/tinoosan/agen8/internal/services/question/infra"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
	taskinfra "github.com/tinoosan/agen8/internal/services/task/infra"
	userapp "github.com/tinoosan/agen8/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8/internal/services/user/domain"
	userinfra "github.com/tinoosan/agen8/internal/services/user/infra"
	implstore "github.com/tinoosan/agen8/internal/store"
)

// Application is the process-scoped composition root for the stripped
// MCP-first work-context server.
type Application struct {
	AuthSvc         *authapp.Service
	UserSvc         *userapp.Service
	CredentialSvc   *credentialapp.Service
	GraphSvc        *graphapp.Service
	GraphLinks      contextlink.Repository
	TaskSvc         *taskapp.Service
	MissionSvc      *missionapp.Service
	ProjectSvc      *projectapp.Service
	FileSvc         *fileapp.Service
	LocationSvc     *locationapp.Service
	EventBus        *eventbus.Bus
	DecisionSvc     *decisionapp.Service
	QuestionSvc     *questionapp.Service
	PinSvc          *pinapp.Service
	NotificationSvc *notificationapp.Service
	LastSeenStore   *lastseen.Store
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
		taskSvc,
		missionLinkedTaskLoader{},
		missionEventPublisher{bus: bus},
		logger.With("service", "mission"),
	)
	if err != nil {
		return nil, fmt.Errorf("build mission service: %w", err)
	}

	graphLinks := &graphServiceLinkPort{}
	taskSvc.SetKeyResultMissionReader(missionRefResolver{missions: missionSvc})
	taskSvc.SetEventPublisher(bus)

	decisionRepo := decisioninfra.NewSQLiteRepository(handle)
	decisionSvc, err := decisionapp.NewService(
		decisionRepo,
		decisiondomain.SystemClock{},
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

	questionRepo, err := questioninfra.NewSQLiteRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build question repository: %w", err)
	}
	questionSvc, err := questionapp.NewService(questionapp.Config{
		Questions: questionRepo,
		Clock:     questiondomain.SystemClock{},
		Events:    bus,
		Decisions: newQuestionDecisionLogger(decisionSvc),
		Logger:    logger.With("service", "question"),
	})
	if err != nil {
		return nil, fmt.Errorf("build question service: %w", err)
	}

	graphSvc, err := graphapp.NewService(
		contextLinkRepo,
		graphapp.DefaultHydrators(
			graphTaskHydrationReader{tasks: taskSvc},
			graphDecisionHydrationReader{decisions: decisionSvc},
			graphMissionHydrationReader{missions: missionSvc},
		),
		2*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("build graph service: %w", err)
	}
	graphLinks.svc = graphSvc
	// The graph derives its structural skeleton (mission -> KR -> task ->
	// decision lineage) from entity refs at read time, so structure is owned by
	// the backend and never re-derived by each consumer.
	graphSvc.SetStructuralResolver(graphapp.NewStructuralResolver(
		graphTaskHydrationReader{tasks: taskSvc},
		graphDecisionHydrationReader{decisions: decisionSvc},
		graphMissionHydrationReader{missions: missionSvc},
	))

	pinRepo, err := pininfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build pin repository: %w", err)
	}
	pinSvc, err := pinapp.NewService(pinapp.Config{Pins: pinRepo, Events: bus})
	if err != nil {
		return nil, fmt.Errorf("build pin service: %w", err)
	}
	// Pinned nodes rank higher in graph_query search; the graph service reads
	// the project's pinned refs through this adapter at query time.
	graphSvc.SetPinReader(pinNodeRefReader{pins: pinSvc})

	// Notifications are a derived projection over the task snapshot — see the
	// notification domain package. The service reads tasks through a thin
	// adapter and reconciles into the notifications table on demand.
	notificationRepo := notificationinfra.NewSQLiteRepository(handle)
	notificationSvc, err := notificationapp.NewService(
		notificationRepo,
		notificationTaskSource{tasks: taskSvc},
		notificationdomain.SystemClock{},
		notificationdomain.DefaultDeriveConfig(),
		logger.With("service", "notification"),
	)
	if err != nil {
		return nil, fmt.Errorf("build notification service: %w", err)
	}

	lastSeenStore, err := lastseen.NewStore(handle)
	if err != nil {
		return nil, fmt.Errorf("build last-seen store: %w", err)
	}

	return &Application{
		AuthSvc:         authSvc,
		UserSvc:         userSvc,
		CredentialSvc:   credentialSvc,
		GraphSvc:        graphSvc,
		GraphLinks:      contextLinkRepo,
		TaskSvc:         taskSvc,
		MissionSvc:      missionSvc,
		ProjectSvc:      projectSvc,
		FileSvc:         fileSvc,
		LocationSvc:     locationSvc,
		EventBus:        bus,
		DecisionSvc:     decisionSvc,
		QuestionSvc:     questionSvc,
		PinSvc:          pinSvc,
		NotificationSvc: notificationSvc,
		LastSeenStore:   lastSeenStore,
	}, nil
}

// notificationTaskSource adapts the task service to the notification app's
// TaskSource port, projecting task.Task onto the neutral TaskSnapshot the
// derivation reads. This is the seam that keeps the notification package from
// importing the task domain.
type notificationTaskSource struct {
	tasks *taskapp.Service
}

func (a notificationTaskSource) Tasks(ctx context.Context, projectID string) ([]notificationdomain.TaskSnapshot, error) {
	tasks, err := a.tasks.List(ctx, taskdomain.TaskFilter{ProjectID: types.ProjectID(strings.TrimSpace(projectID))})
	if err != nil {
		return nil, err
	}
	out := make([]notificationdomain.TaskSnapshot, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, notificationdomain.TaskSnapshot{
			ID:          string(t.ID),
			ProjectID:   string(t.ProjectID),
			Title:       t.Title,
			Status:      string(t.Status),
			CreatedAt:   t.CreatedAt,
			StartedAt:   t.StartedAt,
			CompletedAt: t.CompletedAt,
			UpdatedAt:   t.UpdatedAt,
			ClaimedBy:   string(t.ClaimedByMemberID),
		})
	}
	return out, nil
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

func (a linkTokenIssuerAdapter) ListLinkTokens(ctx context.Context, projectID string) ([]projectapp.LinkTokenSummary, error) {
	if a.auth == nil {
		return nil, fmt.Errorf("auth service is required")
	}
	records, err := a.auth.ListLinkTokens(ctx, authapp.ListLinkTokensParams{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	// Wall-clock now is the right basis for active/expired here: link-token
	// expiry is real elapsed time, and this composition-root adapter is the layer
	// that owns translation between the two services.
	now := time.Now().UTC()
	summaries := make([]projectapp.LinkTokenSummary, 0, len(records))
	for _, lt := range records {
		summaries = append(summaries, projectapp.LinkTokenSummary{
			ID:          lt.ID.String(),
			Prefix:      lt.Prefix,
			ProjectID:   lt.ProjectID,
			WorkspaceID: lt.WorkspaceID,
			Label:       lt.Label,
			ExpiresAt:   lt.ExpiresAt,
			RevokedAt:   lt.RevokedAt,
			CreatedAt:   lt.CreatedAt,
			Active:      lt.IsActive(now),
		})
	}
	return summaries, nil
}

func (a linkTokenIssuerAdapter) RevokeLinkToken(ctx context.Context, tokenID string) error {
	if a.auth == nil {
		return fmt.Errorf("auth service is required")
	}
	id, err := linktoken.NewID(tokenID)
	if err != nil {
		return fmt.Errorf("link token id: %w", err)
	}
	return a.auth.RevokeLinkToken(ctx, id)
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

// pinNodeRefReader adapts the pin service to the graph service's PinReader port
// so pinned nodes can be prioritized in graph_query search results.
type pinNodeRefReader struct {
	pins *pinapp.Service
}

func (r pinNodeRefReader) PinnedNodeRefs(ctx context.Context, projectID string) (map[string]struct{}, error) {
	// The graph service treats an unset pin service as "no pinned refs" so queries
	// remain available even during partial startup or degraded pin wiring.
	if r.pins == nil {
		return nil, nil
	}
	pins, err := r.pins.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	refs := make(map[string]struct{}, len(pins))
	for _, pin := range pins {
		ref := strings.TrimSpace(pin.NodeRef)
		if ref == "" {
			continue
		}
		refs[ref] = struct{}{}
	}
	return refs, nil
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
	case string(missionapp.MissionEventCreated),
		string(missionapp.MissionEventActivated),
		string(missionapp.MissionEventPaused),
		string(missionapp.MissionEventCompleted),
		string(missionapp.MissionEventArchived):
		return p.bus.Publish(eventbus.TopicMissionLifecycle, event)
	default:
		return p.bus.Publish(eventbus.TopicKRProgress, event)
	}
}

var _ slog.Handler
