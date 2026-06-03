package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/caller"
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
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	harnessdomain "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessinfra "github.com/tinoosan/agen8-mcp-server/internal/services/harness/infra"
	humaninputapp "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/app"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	humaninputinfra "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/infra"
	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
	locationinfra "github.com/tinoosan/agen8-mcp-server/internal/services/location/infra"
	messageapp "github.com/tinoosan/agen8-mcp-server/internal/services/message/app"
	messagedomain "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	messageconversation "github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	messageinfra "github.com/tinoosan/agen8-mcp-server/internal/services/message/infra"
	missionapp "github.com/tinoosan/agen8-mcp-server/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	missioninfra "github.com/tinoosan/agen8-mcp-server/internal/services/mission/infra"
	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	operatorinfra "github.com/tinoosan/agen8-mcp-server/internal/services/operator/infra"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	scheduledomain "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	scheduleinfra "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/infra"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	spaceinfra "github.com/tinoosan/agen8-mcp-server/internal/services/space/infra"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	taskinfra "github.com/tinoosan/agen8-mcp-server/internal/services/task/infra"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	userdomain "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	userinfra "github.com/tinoosan/agen8-mcp-server/internal/services/user/infra"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// Application is the single composition root for process-scoped services.
//
// Services live here, are constructed once in NewApplication, and are
// referenced by every layer that needs them (RPCServer, MCPServer,
// RPC handlers). Production code must never
// reconstruct a service that already lives on the Application — doing
// so produces silent duplication where different call sites see
// different instances and per-service state (caches, hooks, in-process
// channels) diverges.
//
// Hard rule: Application has no methods. Only NewApplication and field
// access. If you find yourself wanting to add a method, the logic
// belongs in one of the services Application holds.
//
// Hosted multi-tenant code builds a separate Application per user scope
// via NewApplication with the per-user config; see hosted_rpc_scope.go.
type Application struct {
	AuthSvc       *authapp.Service
	UserSvc       *userapp.Service
	SpaceSvc      *spaceapp.Service
	CredentialSvc *credentialapp.Service
	GraphSvc      *graphapp.Service
	GraphLinks    contextlink.Repository
	MessageSvc    *messageapp.Service
	TaskSvc       *taskapp.Service
	ScheduleSvc   *scheduleapp.Service
	MissionSvc    *missionapp.Service
	ProjectSvc    *projectapp.Service
	FileSvc       *fileapp.Service
	LocationSvc   *locationapp.Service
	HarnessSvc    *harnessapp.Service
	OperatorSvc   *operatorapp.Service
	EventBus      *eventbus.Bus

	DecisionSvc          *decisionapp.Service
	HumanInputSvc        *humaninputapp.Service
	HumanInputWake       *humaninputapp.MemoryWakeRegistry
	HumanInputMCPAwaiter *humaninputapp.Awaiter
}

// NewApplication builds the process-scoped service graph.
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
	credentialSvc, err := credentialapp.NewService(credentialapp.Config{
		Repository: credentialRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("build credential service: %w", err)
	}

	contextLinkRepo, err := contextlink.NewSQLiteRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build context link repository: %w", err)
	}
	bus := eventbus.New(nil)

	spaceRepo, err := spaceinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build space repository: %w", err)
	}
	memberRepo, err := spaceinfra.NewMemberRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build space member repository: %w", err)
	}
	spaceSvc, err := spaceapp.NewService(
		spaceRepo,
		memberRepo,
		spacedomain.SystemClock{},
		caller.ContextResolver{},
		harnessdomain.DefaultCatalog(),
		bus,
		logger.With("service", "space"),
	)
	if err != nil {
		return nil, fmt.Errorf("build space service: %w", err)
	}

	harnessRepo, err := harnessinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build harness session repository: %w", err)
	}
	harnessRunRepo, err := harnessinfra.NewRunRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build harness run repository: %w", err)
	}
	lostRuns, err := harnessRunRepo.MarkRuntimeLost(context.Background())
	if err != nil {
		return nil, fmt.Errorf("mark stale harness runs runtime lost: %w", err)
	}
	if len(lostRuns) > 0 {
		logger.Warn("marked stale harness runs as failed", "count", len(lostRuns))
	}
	harnessSvc, err := harnessapp.NewService(
		harnessdomain.DefaultCatalog(),
		defaultHarnessRuntimes(),
		harnessRepo,
		harnessRunRepo,
		func() string { return "session_" + uuid.NewString() },
		func() time.Time { return time.Now().UTC() },
		logger.With("service", "harness"),
	)
	if err != nil {
		return nil, fmt.Errorf("build harness service: %w", err)
	}
	harnessSvc.SetMCPConfigFormatter(harnessinfra.MCPConfigFormatter{DataDir: cfg.Host.DataDir})

	messageRepo, err := messageinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build message repository: %w", err)
	}
	messageConversationRepo, err := messageinfra.NewConversationRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build message conversation repository: %w", err)
	}
	messageSvc, err := messageapp.NewService(messageapp.NewServiceParams{
		Repository:             messageRepo,
		Conversations:          messageConversationRepo,
		HarnessChatSender:      harnessChatSenderAdapter{svc: harnessSvc},
		AutoStartAgentDelivery: true,
		Logger:                 logger.With("service", "message"),
		Clock:                  messageapp.SystemClock{},
	})
	if err != nil {
		return nil, fmt.Errorf("build message service: %w", err)
	}
	harnessSvc.SetExternalSessionEventSink(harnessExternalEventSinkAdapter{svc: messageSvc})

	projectRepo, err := projectinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build project repository: %w", err)
	}
	projectClusterRepo, err := projectinfra.NewClusterRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build project cluster repository: %w", err)
	}
	projectSvc, err := projectapp.NewService(projectapp.Config{
		Projects: projectRepo,
		Clusters: projectClusterRepo,
		Spaces:   spaceSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("build project service: %w", err)
	}
	harnessSvc.SetProjectWorkdirResolver(harnessProjectWorkdirResolver{projects: projectSvc})
	messageSvc.SetProjectRootResolver(messageProjectRootResolver{projects: projectSvc, dataDir: cfg.Host.DataDir})

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
	locationTransport := locationinfra.NewTransport(locationinfra.TransportConfig{
		Credentials:     credentialSvc,
		LocalDaemonAddr: cfg.DaemonHTTPAddr,
		Logger:          logger.With("service", "file.location.transport"),
	})
	fileRepo, err := fileinfra.NewLocationRepository(locationRepo, locationTransport)
	if err != nil {
		return nil, fmt.Errorf("build file repository: %w", err)
	}
	fileSvc, err := fileapp.NewService(fileapp.Config{
		Files:    fileRepo,
		Projects: projectSvc,
	})
	if err != nil {
		return nil, fmt.Errorf("build file service: %w", err)
	}
	harnessSvc.SetRuntimeHostResolver(harnessRuntimeHostResolver{locations: locationSvc})
	harnessSvc.SetAttachmentStager(harnessAttachmentStager{locations: locationSvc})

	taskRepo, err := taskinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build task repository: %w", err)
	}
	taskSvc, err := taskapp.NewService(
		taskRepo,
		taskdomain.SystemClock{},
		caller.ContextResolver{},
		spaceSvc,
		spaceSvc,
		messageSvc,
		logger.With("service", "task"),
	)
	if err != nil {
		return nil, fmt.Errorf("build task service: %w", err)
	}
	messageSvc.SetTaskStateReader(taskSvc)
	scheduleRepo, err := scheduleinfra.NewRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build schedule repository: %w", err)
	}
	scheduleSvc, err := scheduleapp.NewService(scheduleRepo, scheduledomain.SystemClock{}, logger.With("service", "schedule"))
	if err != nil {
		return nil, fmt.Errorf("build schedule service: %w", err)
	}
	taskCreateExecutor, err := scheduleapp.NewTaskCreateExecutor(taskSvc)
	if err != nil {
		return nil, fmt.Errorf("build schedule task.create executor: %w", err)
	}
	if err := scheduleSvc.RegisterExecutor(scheduledomain.TargetKindTaskCreate, taskCreateExecutor); err != nil {
		return nil, fmt.Errorf("register schedule task.create executor: %w", err)
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
		spaceSvc,
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
	taskSvc.SetKeyResultMissionReader(operatorMissionRefResolver{missions: missionSvc})

	decisionRepo := decisioninfra.NewSQLiteRepository(handle)
	humanInputRepo, err := humaninputinfra.NewSQLiteRepository(handle)
	if err != nil {
		return nil, fmt.Errorf("build human input repository: %w", err)
	}
	humanInputSvc, err := humaninputapp.NewService(
		humanInputRepo,
		humaninputdomain.SystemClock{},
		func() string { return "hi_" + uuid.NewString() },
		humaninputdomain.DefaultValidator{},
		logger.With("service", "humaninput"),
	)
	if err != nil {
		return nil, fmt.Errorf("build human input service: %w", err)
	}
	humanInputWake := humaninputapp.NewMemoryWakeRegistry()
	humanInputAwaiter, err := humaninputapp.NewAwaiter(humanInputSvc, humanInputWake)
	if err != nil {
		return nil, fmt.Errorf("build human input awaiter: %w", err)
	}
	harnessSvc.SetHumanInputAwaiter(humanInputAwaiter)
	decisionSvc, err := decisionapp.NewService(
		decisionRepo,
		decisiondomain.SystemClock{},
		graphLinks,
		graphLinks,
		bus,
		nil,
		nil,
		spaceSvc,
		logger.With("service", "decision"),
	)
	if err != nil {
		return nil, fmt.Errorf("build decision service: %w", err)
	}
	operatorStore, err := operatorinfra.NewSQLiteStore(handle)
	if err != nil {
		return nil, fmt.Errorf("build operator repository: %w", err)
	}
	operatorMessages := operatorMessagePublisher{messages: messageSvc}
	operatorSvc, err := operatorapp.NewService(
		operatorStore,
		operatorStore,
		operatorTaskPort{tasks: taskSvc},
		graphLinks,
		operatorEventPublisher{bus: bus},
		operatorDecisionCreator{decisions: decisionSvc},
		operatorMessages,
		operatorMissionRefResolver{missions: missionSvc},
		logger.With("service", "operator"),
	)
	if err != nil {
		return nil, fmt.Errorf("build operator service: %w", err)
	}
	graphSvc, err := graphapp.NewService(
		contextLinkRepo,
		graphapp.DefaultHydrators(taskSvc, decisionSvc, missionSvc, operatorSvc, ""),
		2*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("build graph service: %w", err)
	}
	graphLinks.svc = graphSvc

	return &Application{
		AuthSvc:              authSvc,
		UserSvc:              userSvc,
		SpaceSvc:             spaceSvc,
		CredentialSvc:        credentialSvc,
		GraphSvc:             graphSvc,
		GraphLinks:           contextLinkRepo,
		MessageSvc:           messageSvc,
		TaskSvc:              taskSvc,
		ScheduleSvc:          scheduleSvc,
		MissionSvc:           missionSvc,
		ProjectSvc:           projectSvc,
		FileSvc:              fileSvc,
		LocationSvc:          locationSvc,
		HarnessSvc:           harnessSvc,
		OperatorSvc:          operatorSvc,
		EventBus:             bus,
		DecisionSvc:          decisionSvc,
		HumanInputSvc:        humanInputSvc,
		HumanInputWake:       humanInputWake,
		HumanInputMCPAwaiter: humanInputAwaiter,
	}, nil
}

type harnessChatSenderAdapter struct {
	svc *harnessapp.Service
}

type harnessExternalEventSinkAdapter struct {
	svc *messageapp.Service
}

type messageProjectRootResolver struct {
	projects *projectapp.Service
	dataDir  string
}

func (r messageProjectRootResolver) ResolveProjectRoot(ctx context.Context, projectID types.ProjectID) (string, error) {
	if r.projects == nil {
		return "", fmt.Errorf("project service is required")
	}
	project, err := r.projects.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("load message attachment project %s: %w", projectID, err)
	}
	root := strings.TrimSpace(r.dataDir)
	if root == "" {
		root = strings.TrimSpace(project.Root())
	}
	if root == "" {
		return "", fmt.Errorf("message attachment storage root is required")
	}
	return filepath.Clean(root), nil
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

type operatorTaskPort struct {
	tasks *taskapp.Service
}

func (p operatorTaskPort) BlockTask(ctx context.Context, taskID string, blockerID string) error {
	if p.tasks == nil {
		return fmt.Errorf("task service is required")
	}
	taskID = strings.TrimSpace(taskID)
	blockerID = strings.TrimSpace(blockerID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	if blockerID == "" {
		return fmt.Errorf("blocker id is required")
	}
	_, err := p.tasks.Block(ctx, taskdomain.TaskID(taskID), "blocked by operator item "+blockerID)
	return err
}

func (p operatorTaskPort) UnblockTask(ctx context.Context, taskID string, blockerID string) error {
	if p.tasks == nil {
		return fmt.Errorf("task service is required")
	}
	taskID = strings.TrimSpace(taskID)
	blockerID = strings.TrimSpace(blockerID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	if blockerID == "" {
		return fmt.Errorf("blocker id is required")
	}
	_, err := p.tasks.Unblock(ctx, taskdomain.TaskID(taskID), "operator item "+blockerID+" resolved")
	return err
}

type operatorEventPublisher struct {
	bus *eventbus.Bus
}

func (p operatorEventPublisher) PublishOperatorEvent(_ context.Context, event any) error {
	if p.bus == nil {
		return fmt.Errorf("operator event bus is required")
	}
	switch event.(type) {
	case eventbus.OALifecycleEvent:
		return p.bus.Publish(eventbus.TopicOALifecycle, event)
	case eventbus.EscalationLifecycleEvent:
		return p.bus.Publish(eventbus.TopicEscalationLifecycle, event)
	default:
		return fmt.Errorf("unsupported operator event type %T", event)
	}
}

type operatorDecisionCreator struct {
	decisions *decisionapp.Service
}

func (p operatorDecisionCreator) CreateDecision(ctx context.Context, decision decisiondomain.Decision) (decisiondomain.DecisionID, error) {
	if p.decisions == nil {
		return "", fmt.Errorf("decision service is required")
	}
	created, err := p.decisions.Create(ctx, decision)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (p operatorTaskPort) GetTaskKeyResultRef(ctx context.Context, taskRef string) (string, error) {
	if p.tasks == nil {
		return "", fmt.Errorf("task service is required")
	}
	taskRef = strings.TrimSpace(taskRef)
	if taskRef == "" {
		return "", fmt.Errorf("task ref is required")
	}
	task, err := p.tasks.Get(ctx, taskdomain.TaskID(taskRef))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(task.KeyResultRef), nil
}

type operatorMissionRefResolver struct {
	missions *missionapp.Service
}

func (r operatorMissionRefResolver) GetMissionFromKeyResult(ctx context.Context, krRef string) (string, error) {
	return r.KeyResultMission(ctx, krRef)
}

func (r operatorMissionRefResolver) KeyResultMission(ctx context.Context, krRef string) (string, error) {
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

func (r operatorMissionRefResolver) ValidateMission(ctx context.Context, missionID string) error {
	if r.missions == nil {
		return fmt.Errorf("mission service is required")
	}
	missionID = strings.TrimSpace(missionID)
	if missionID == "" {
		return fmt.Errorf("mission id is required")
	}
	_, err := r.missions.GetMission(ctx, missiondomain.MissionID(missionID))
	return err
}

type operatorMessagePublisher struct {
	messages *messageapp.Service
}

func (p operatorMessagePublisher) ResolveEscalation(ctx context.Context, esc operatordomain.Escalation, params operatorapp.ResolveEscalationParams) error {
	return p.publishSystem(ctx, operatorSystemMessage{
		SpaceID:       esc.SpaceID,
		MemberID:      esc.MemberID,
		TaskRef:       esc.TaskRef,
		Subject:       "Escalation resolved",
		BodyText:      operatorapp.FormatEscalationResolutionMessage(esc, params),
		CorrelationID: string(esc.ID),
	})
}

func (p operatorMessagePublisher) CompleteAction(ctx context.Context, action operatordomain.OperatorAction) error {
	return p.publishSystem(ctx, operatorSystemMessage{
		SpaceID:       action.SpaceID,
		MemberID:      action.MemberID,
		TaskRef:       action.TaskRef,
		Subject:       "Operator action completed",
		BodyText:      operatorapp.FormatCompletionMessage(action),
		CorrelationID: string(action.ID),
	})
}

func (p operatorMessagePublisher) CommentOnAction(ctx context.Context, action operatordomain.OperatorAction, comment operatordomain.Comment) error {
	return p.publishSystem(ctx, operatorSystemMessage{
		SpaceID:       action.SpaceID,
		MemberID:      action.MemberID,
		TaskRef:       action.TaskRef,
		Subject:       "Operator comment",
		BodyText:      operatorapp.FormatCommentMessage(action, comment),
		CorrelationID: string(action.ID),
	})
}

func (p operatorMessagePublisher) BlockAction(ctx context.Context, action operatordomain.OperatorAction, reason string) error {
	return p.publishSystem(ctx, operatorSystemMessage{
		SpaceID:       action.SpaceID,
		MemberID:      action.MemberID,
		TaskRef:       action.TaskRef,
		Subject:       "Operator action blocked",
		BodyText:      operatorapp.FormatBlockedMessage(action, reason),
		CorrelationID: string(action.ID),
	})
}

type operatorSystemMessage struct {
	SpaceID       string
	MemberID      string
	TaskRef       string
	Subject       string
	BodyText      string
	CorrelationID string
}

func (p operatorMessagePublisher) publishSystem(ctx context.Context, msg operatorSystemMessage) error {
	if p.messages == nil {
		return fmt.Errorf("message service is required")
	}
	spaceID := strings.TrimSpace(msg.SpaceID)
	memberID := strings.TrimSpace(msg.MemberID)
	if spaceID == "" {
		return fmt.Errorf("operator message space id is required")
	}
	if memberID == "" {
		return fmt.Errorf("operator message member id is required")
	}
	body := strings.TrimSpace(msg.BodyText)
	if body == "" {
		return fmt.Errorf("operator message body is required")
	}
	_, err := p.messages.PublishAgentMessage(ctx, messagedomain.NewMessageInput{
		Route: messagedomain.MessageRoute{
			SpaceID:             spacedomain.SpaceID(spaceID),
			DestinationMemberID: member.ID(memberID),
		},
		Content: messagedomain.MessageContent{
			Kind:    types.AgentMessageKindSystem,
			Subject: strings.TrimSpace(msg.Subject),
			Body: map[string]any{
				"text": body,
			},
			TaskRef: taskdomain.TaskID(strings.TrimSpace(msg.TaskRef)),
		},
		Producer: messagedomain.MessageProducer{
			IntentID:      types.IntentID("operator.message:" + uuid.NewString()),
			CorrelationID: types.CorrelationID(strings.TrimSpace(msg.CorrelationID)),
			Producer:      "operator",
		},
	})
	return err
}

func (a harnessChatSenderAdapter) SendMessage(ctx context.Context, input messageapp.HarnessChatMessage) (messageapp.HarnessChatResult, error) {
	if a.svc == nil {
		return messageapp.HarnessChatResult{}, fmt.Errorf("harness service is required")
	}
	result, err := a.svc.SendMessage(ctx, harnessapp.SendMessageParams{
		SpaceID:               input.SpaceID,
		MemberID:              input.MemberID,
		ChannelID:             input.ChannelID,
		ConversationMessageID: input.ConversationMessageID,
		SenderType:            input.SenderType,
		SenderID:              input.SenderID,
		Text:                  input.Text,
		Attachments:           harnessAttachmentsFromMessage(input.Attachments),
		AllowSteering:         input.AllowSteering,
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

func (a harnessExternalEventSinkAdapter) AppendHarnessExternalEvent(ctx context.Context, event harnessapp.ExternalSessionEvent) error {
	if a.svc == nil {
		return fmt.Errorf("message service is required")
	}
	var activity *messageapp.HarnessActivity
	if event.Activity != nil {
		activity = &messageapp.HarnessActivity{
			SessionID:  event.Activity.SessionID,
			TurnID:     event.Activity.TurnID,
			ToolCallID: event.Activity.ToolCallID,
			ToolName:   event.Activity.ToolName,
			Sequence:   event.Activity.Sequence,
			Status:     event.Activity.Status,
			Text:       event.Activity.Text,
			Data:       event.Activity.Data,
		}
	}
	return a.svc.AppendHarnessExternalEvent(ctx, messageapp.HarnessExternalEvent{
		SpaceID:    event.SpaceID,
		ChannelID:  event.ChannelID,
		MemberID:   event.MemberID,
		SessionID:  event.SessionID,
		SessionRef: event.SessionRef,
		TurnID:     event.TurnID,
		Sequence:   event.Sequence,
		Text:       event.Text,
		Thinking:   event.Thinking,
		Data:       event.Data,
		Activity:   activity,
		Completed:  event.Completed,
	})
}

func harnessAttachmentsFromMessage(attachments []messageconversation.Attachment) []harnessapp.PromptAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]harnessapp.PromptAttachment, 0, len(attachments))
	for _, attachment := range attachments {
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

type projectLocationChecker struct {
	projects *projectapp.Service
}

type harnessProjectWorkdirResolver struct{ projects *projectapp.Service }

func (r harnessProjectWorkdirResolver) ResolveHarnessWorkdir(ctx context.Context, projectID string) (harnessapp.ProjectWorkdir, error) {
	if r.projects == nil {
		return harnessapp.ProjectWorkdir{}, fmt.Errorf("project service is required")
	}
	project, err := r.projects.GetProject(ctx, types.ProjectID(projectID))
	if err != nil {
		return harnessapp.ProjectWorkdir{}, fmt.Errorf("load harness project %s: %w", projectID, err)
	}
	root := strings.TrimSpace(project.Root())
	if root == "" {
		return harnessapp.ProjectWorkdir{}, fmt.Errorf("project %s root is required", projectID)
	}
	locationID := strings.TrimSpace(string(project.LocationID()))
	if locationID == "" {
		locationID = "local"
	}
	return harnessapp.ProjectWorkdir{LocationID: locationID, Workdir: root}, nil
}

type harnessRuntimeHostResolver struct{ locations *locationapp.Service }

func (r harnessRuntimeHostResolver) ResolveRuntimeHost(ctx context.Context, input harnessapp.RuntimeHostRequest) (harnessapp.RuntimeHost, error) {
	locationID := strings.TrimSpace(input.LocationID)
	if locationID == "" || locationID == "local" {
		return harnessapp.RuntimeHost{}, nil
	}
	if r.locations == nil {
		return harnessapp.RuntimeHost{}, fmt.Errorf("location service is required")
	}
	host, err := r.locations.EnsureBridge(ctx, locationdomain.ID(locationID))
	if err != nil {
		return harnessapp.RuntimeHost{}, err
	}
	ws := strings.TrimRight(strings.TrimSpace(host.WebSocketURL), "/")
	switch strings.ToLower(strings.TrimSpace(input.HarnessKind)) {
	case "codex":
		ws += "/codex"
	case "claude-cli":
		ws += "/claude"
	}
	return harnessapp.RuntimeHost{
		AppServerURL: ws,
		MCPBaseURL:   strings.TrimSpace(host.MCPBaseURL),
		Diagnostics:  strings.TrimSpace(host.Diagnostics),
	}, nil
}

type harnessAttachmentStager struct{ locations *locationapp.Service }

func (s harnessAttachmentStager) StageAttachments(ctx context.Context, request harnessapp.AttachmentStageRequest) ([]harnessapp.PromptAttachment, error) {
	attachments := append([]harnessapp.PromptAttachment(nil), request.Attachments...)
	locationID := strings.TrimSpace(request.LocationID)
	if len(attachments) == 0 || locationID == "" || locationID == "local" {
		return attachments, nil
	}
	if s.locations == nil {
		return nil, fmt.Errorf("location service is required")
	}
	workdir := strings.TrimSpace(request.Workdir)
	if workdir == "" {
		return nil, fmt.Errorf("remote attachment workdir is required")
	}
	for i := range attachments {
		staged, err := s.stageAttachment(ctx, locationdomain.ID(locationID), workdir, attachments[i])
		if err != nil {
			return nil, err
		}
		attachments[i] = staged
	}
	return attachments, nil
}

func (s harnessAttachmentStager) stageAttachment(ctx context.Context, locationID locationdomain.ID, workdir string, attachment harnessapp.PromptAttachment) (harnessapp.PromptAttachment, error) {
	localPath := strings.TrimSpace(attachment.URI)
	if localPath == "" {
		return harnessapp.PromptAttachment{}, fmt.Errorf("attachment %s URI is required", attachment.ID)
	}
	in, err := os.Open(localPath)
	if err != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("open attachment %s: %w", attachment.ID, err)
	}
	defer in.Close()
	remoteDir := pathpkg.Join(workdir, ".agen8", "conversation-attachments", "runtime")
	remotePath := pathpkg.Join(remoteDir, remoteAttachmentFileName(attachment))
	proc, err := s.locations.StartCommand(ctx, locationID, locationapp.CommandSpec{
		Command: "sh",
		Args: []string{
			"-lc",
			`set -e; mkdir -p "$1"; base64 -d > "$2"`,
			"agen8-stage-attachment",
			remoteDir,
			remotePath,
		},
		Workdir: workdir,
	})
	if err != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("start remote attachment stage for %s: %w", attachment.ID, err)
	}
	stdin, err := proc.StdinPipe()
	if err != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("open remote attachment stdin for %s: %w", attachment.ID, err)
	}
	if err := proc.Start(); err != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("start remote attachment command for %s: %w", attachment.ID, err)
	}
	encoder := base64.NewEncoder(base64.StdEncoding, stdin)
	_, copyErr := io.Copy(encoder, in)
	closeErr := encoder.Close()
	stdinCloseErr := stdin.Close()
	if copyErr != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("encode attachment %s: %w", attachment.ID, copyErr)
	}
	if closeErr != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("close attachment encoder %s: %w", attachment.ID, closeErr)
	}
	if stdinCloseErr != nil {
		return harnessapp.PromptAttachment{}, fmt.Errorf("close remote attachment stdin %s: %w", attachment.ID, stdinCloseErr)
	}
	if err := proc.Wait(); err != nil {
		stderr := strings.TrimSpace(proc.StderrText())
		if stderr != "" {
			return harnessapp.PromptAttachment{}, fmt.Errorf("stage remote attachment %s: %w: %s", attachment.ID, err, stderr)
		}
		return harnessapp.PromptAttachment{}, fmt.Errorf("stage remote attachment %s: %w", attachment.ID, err)
	}
	attachment.URI = remotePath
	return attachment, nil
}

func remoteAttachmentFileName(attachment harnessapp.PromptAttachment) string {
	id := sanitizeRemotePathSegment(attachment.ID)
	if id == "" {
		id = "attachment"
	}
	name := sanitizeRemotePathSegment(attachment.Name)
	if name == "" {
		name = "upload"
	}
	return id + "-" + name
}

func sanitizeRemotePathSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, value)
	return strings.Trim(value, ".-")
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
