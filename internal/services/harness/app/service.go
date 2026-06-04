package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type IDGenerator func() string
type Clock func() time.Time

type SendMessageParams struct {
	SpaceID               string
	MemberID              string
	ChannelID             string
	ConversationMessageID string
	SenderType            string
	SenderID              string
	Text                  string
	Attachments           []PromptAttachment
	AllowSteering         bool
	SteerOnly             bool
	OnAssistantDelta      func(ctx context.Context, delta AssistantDelta) error
	OnThinkingDelta       func(ctx context.Context, delta ThinkingDelta) error
	OnActivity            func(ctx context.Context, activity ActivityEvent) error
}

const errNoActiveHarnessRunToSteer = "no active harness run to steer"

type PromptAttachment struct {
	ID        string
	Name      string
	MediaType string
	SizeBytes int64
	URI       string
}

type SendMessageResult struct {
	SessionID string
	RunID     string
	TurnID    string
	Delivery  string
	Text      string
}

type AssistantDelta struct {
	SessionID string
	TurnID    string
	Sequence  int
	Text      string
}

type ThinkingDelta struct {
	SessionID string
	TurnID    string
	Sequence  int
	Text      string
	Data      map[string]string
}

type ActivityEvent struct {
	SessionID  string
	TurnID     string
	ToolCallID string
	ToolName   string
	Sequence   int
	Status     string
	Text       string
	Data       map[string]string
}

type RunListParams struct {
	ProjectID string
	SpaceID   string
	ChannelID string
	MemberID  string
	SessionID string
	Status    []string
	Limit     int
}

type RequestStopParams struct {
	RunID       string
	TurnID      string
	ChannelID   string
	RequestedBy string
}

type ActivateSessionParams struct {
	ProjectID      string
	MemberID       string
	SpaceID        string
	ChannelID      string
	DisplayName    string
	MemberType     string
	LifecycleState string
	HarnessKind    string
	Model          string
	Effort         string
	PermissionMode string
	ConfigRef      string
	SessionRef     string
	MCPToken       string
	MCPServers     []string
}

type MCPProvisioner interface {
	ProvisionMCP(ctx context.Context, p ActivateSessionParams) (MCPProvisioning, error)
}

type MCPProvisioning struct {
	Token string
	URL   string
}

type MCPConfigFormatter interface {
	FormatMCPServers(ctx context.Context, request MCPConfigRequest) ([]string, error)
}

type MCPConfigRequest struct {
	HarnessKind string
	RawURL      string
	BaseURL     string
	Token       string
}

type ProjectWorkdirResolver interface {
	ResolveHarnessWorkdir(ctx context.Context, projectID string) (ProjectWorkdir, error)
}

type ProjectWorkdir struct {
	LocationID string
	Workdir    string
}

type RuntimeHostRequest struct {
	LocationID  string
	HarnessKind string
	SessionID   string
	ProjectID   string
	MemberID    string
	SessionRef  string
	MCPToken    string
}

type RuntimeHost struct {
	AppServerURL string
	MCPBaseURL   string
	Diagnostics  string
}

type RuntimeHostResolver interface {
	ResolveRuntimeHost(ctx context.Context, input RuntimeHostRequest) (RuntimeHost, error)
}

type AttachmentStageRequest struct {
	LocationID  string
	Workdir     string
	Attachments []PromptAttachment
}

type AttachmentStager interface {
	StageAttachments(ctx context.Context, request AttachmentStageRequest) ([]PromptAttachment, error)
}

type HumanInputAwaiter interface {
	Await(ctx context.Context, pending humaninputdomain.PendingRequest) (json.RawMessage, error)
}

type Service struct {
	catalog            *domain.Catalog
	registry           *domain.Registry
	repo               domain.SessionRepository
	runs               harnessrun.Repository
	newID              IDGenerator
	now                Clock
	logger             *slog.Logger
	mcpProvisioner     MCPProvisioner
	mcpConfigFormatter MCPConfigFormatter
	workdirResolver    ProjectWorkdirResolver
	runtimeHosts       RuntimeHostResolver
	attachmentStager   AttachmentStager
	humanInputAwaiter  HumanInputAwaiter
	// Lock order: do not hold cancelMu or steeringMu while acquiring another Service mutex.
	cancelMu        sync.Mutex
	cancelHandles   map[string]context.CancelFunc
	steeringMu      sync.Mutex
	steeringHandles map[string]chan domain.PromptInput
}

func NewService(catalog *domain.Catalog, runtimes []domain.Runtime, repo domain.SessionRepository, runRepo harnessrun.Repository, newID IDGenerator, now Clock, logger *slog.Logger) (*Service, error) {
	if catalog == nil {
		return nil, fmt.Errorf("catalog is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("session repository is required")
	}
	if runRepo == nil {
		return nil, fmt.Errorf("run repository is required")
	}
	if newID == nil {
		return nil, fmt.Errorf("id generator is required")
	}
	if now == nil {
		return nil, fmt.Errorf("clock is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "harness")
	}
	registry, err := domain.NewRegistry(runtimes...)
	if err != nil {
		logger.Error("harness runtime registry rejected", "error", err)
		return nil, fmt.Errorf("build runtime registry: %w", err)
	}
	return &Service{
		catalog:         catalog,
		registry:        registry,
		repo:            repo,
		runs:            runRepo,
		newID:           newID,
		now:             now,
		logger:          logger,
		cancelHandles:   map[string]context.CancelFunc{},
		steeringHandles: map[string]chan domain.PromptInput{},
	}, nil
}

func (s *Service) ValidateConfig(kind, model, effort string) error {
	if err := s.catalog.ValidateConfig(kind, model, effort); err != nil {
		s.logger.Warn("harness config rejected", "harness_kind", kind, "model", model, "effort", effort, "reason", err.Error())
		return err
	}
	return nil
}

func (s *Service) SetHumanInputAwaiter(awaiter HumanInputAwaiter) {
	if s == nil {
		return
	}
	s.humanInputAwaiter = awaiter
}

func (s *Service) ValidateRuntimeConfig(kind, model, effort, permissionMode, configRef string) error {
	if err := s.catalog.ValidateRuntimeConfig(kind, model, effort, permissionMode, configRef); err != nil {
		s.logger.Warn("harness runtime config rejected", "harness_kind", kind, "model", model, "effort", effort, "permission_mode", permissionMode, "reason", err.Error())
		return err
	}
	if strings.TrimSpace(permissionMode) == "codex/custom-config" {
		path := strings.TrimSpace(configRef)
		if _, err := os.ReadFile(path); err != nil {
			return fmt.Errorf("read codex config ref %q: %w", path, err)
		}
	}
	return nil
}

func (s *Service) SupportedHarnesses() []string {
	return s.catalog.SupportedHarnesses()
}

func (s *Service) CatalogEntries() []domain.HarnessEntry {
	return s.catalog.Entries()
}

func (s *Service) SupportedModels(kind string) []string {
	return s.catalog.SupportedModels(kind)
}

func (s *Service) SupportedEfforts(kind, model string) []string {
	return s.catalog.SupportedEfforts(kind, model)
}

func (s *Service) DefaultPermissionMode(kind string) string {
	return s.catalog.DefaultPermissionMode(kind)
}

func (s *Service) DefaultModel(kind string) string {
	models := s.catalog.SupportedModels(kind)
	if len(models) == 0 {
		return ""
	}
	return models[0]
}

func (s *Service) CompatibilityPermissionMode(kind string) string {
	return s.catalog.CompatibilityPermissionMode(kind)
}

func (s *Service) GetRuntime(kind string) (domain.Runtime, error) {
	return s.registry.Get(kind)
}

func (s *Service) SetMCPProvisioner(provisioner MCPProvisioner) {
	s.mcpProvisioner = provisioner
}

func (s *Service) SetMCPConfigFormatter(formatter MCPConfigFormatter) {
	s.mcpConfigFormatter = formatter
}

func (s *Service) SetProjectWorkdirResolver(resolver ProjectWorkdirResolver) {
	s.workdirResolver = resolver
}

func (s *Service) SetRuntimeHostResolver(resolver RuntimeHostResolver) {
	s.runtimeHosts = resolver
}

func (s *Service) SetAttachmentStager(stager AttachmentStager) {
	s.attachmentStager = stager
}

func (s *Service) ListRuns(ctx context.Context, p RunListParams) ([]harnessrun.Run, error) {
	statuses := make([]harnessrun.Status, 0, len(p.Status))
	for _, raw := range p.Status {
		status := harnessrun.Status(strings.TrimSpace(raw))
		if status == "" {
			continue
		}
		if !harnessrun.ValidStatus(status) {
			return nil, fmt.Errorf("invalid run status %q", raw)
		}
		statuses = append(statuses, status)
	}
	rows, err := s.runs.List(ctx, harnessrun.Filter{
		ProjectID: strings.TrimSpace(p.ProjectID),
		SpaceID:   strings.TrimSpace(p.SpaceID),
		ChannelID: strings.TrimSpace(p.ChannelID),
		MemberID:  strings.TrimSpace(p.MemberID),
		SessionID: strings.TrimSpace(p.SessionID),
		Status:    statuses,
		Limit:     p.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list harness runs: %w", err)
	}
	return rows, nil
}

func (s *Service) RequestStop(ctx context.Context, p RequestStopParams) (harnessrun.Run, error) {
	runID := strings.TrimSpace(p.RunID)
	turnID := strings.TrimSpace(p.TurnID)
	if runID == "" && turnID == "" {
		return harnessrun.Run{}, fmt.Errorf("runId or turnId is required")
	}
	var item *harnessrun.Run
	var err error
	if runID != "" {
		item, err = s.runs.Get(ctx, runID)
	} else {
		item, err = s.runs.GetByTurnID(ctx, turnID)
	}
	if err != nil {
		return harnessrun.Run{}, err
	}
	if item == nil {
		return harnessrun.Run{}, fmt.Errorf("harness run not found")
	}
	if channelID := strings.TrimSpace(p.ChannelID); channelID != "" && strings.TrimSpace(item.ChannelID) != channelID {
		return harnessrun.Run{}, fmt.Errorf("harness run %q belongs to channel %q, not %q", item.ID, item.ChannelID, channelID)
	}
	if item.IsTerminal() {
		return *item, nil
	}
	if err := item.RequestStop(strings.TrimSpace(p.RequestedBy), s.now()); err != nil {
		return harnessrun.Run{}, err
	}
	if err := s.runs.Save(ctx, *item); err != nil {
		return harnessrun.Run{}, fmt.Errorf("persist harness run stop request: %w", err)
	}
	cancel := s.cancelHandle(item.ID)
	if cancel == nil {
		return harnessrun.Run{}, fmt.Errorf("harness run %q is active but has no cancel handle", item.ID)
	}
	cancel()
	return *item, nil
}

func (s *Service) cancelHandle(runID string) context.CancelFunc {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	return s.cancelHandles[strings.TrimSpace(runID)]
}

func (s *Service) registerCancelHandle(runID string, cancel context.CancelFunc) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.cancelHandles == nil {
		s.cancelHandles = map[string]context.CancelFunc{}
	}
	s.cancelHandles[strings.TrimSpace(runID)] = cancel
}

func (s *Service) clearCancelHandle(runID string) {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	delete(s.cancelHandles, strings.TrimSpace(runID))
}

func (s *Service) registerSteeringHandle(runID string, steering chan domain.PromptInput) {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	if s.steeringHandles == nil {
		s.steeringHandles = map[string]chan domain.PromptInput{}
	}
	s.steeringHandles[strings.TrimSpace(runID)] = steering
}

func (s *Service) steeringHandle(runID string) chan domain.PromptInput {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	return s.steeringHandles[strings.TrimSpace(runID)]
}

func (s *Service) clearSteeringHandle(runID string) {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	runID = strings.TrimSpace(runID)
	steering := s.steeringHandles[runID]
	delete(s.steeringHandles, runID)
	if steering != nil {
		close(steering)
	}
}

func (s *Service) startRun(ctx context.Context, session *domain.Session, turnID, nativeSessionRef string) (harnessrun.Run, context.Context, context.CancelFunc, chan domain.PromptInput, error) {
	if session == nil {
		return harnessrun.Run{}, nil, nil, nil, fmt.Errorf("session is required")
	}
	if active, err := s.runs.GetActiveBySession(ctx, session.ID); err != nil {
		return harnessrun.Run{}, nil, nil, nil, err
	} else if active != nil {
		if s.cancelHandle(active.ID) != nil {
			return harnessrun.Run{}, nil, nil, nil, fmt.Errorf("harness session %q already has active run %q", session.ID, active.ID)
		}
		if err := active.MarkFailed("runtime_lost", s.now()); err != nil {
			return harnessrun.Run{}, nil, nil, nil, fmt.Errorf("mark stale active harness run %q runtime lost: %w", active.ID, err)
		}
		if err := s.runs.Save(ctx, *active); err != nil {
			return harnessrun.Run{}, nil, nil, nil, fmt.Errorf("persist stale active harness run %q runtime lost: %w", active.ID, err)
		}
		s.logger.WarnContext(ctx, "marked stale active harness run runtime lost",
			"run_id", active.ID,
			"session_id", active.SessionID,
			"space_id", active.SpaceID,
			"member_id", active.MemberID,
		)
	}
	item, err := harnessrun.Start(harnessrun.StartParams{
		ID:               runIDForConversationTurn(turnID),
		ProjectID:        session.ProjectID,
		SpaceID:          session.SpaceID,
		ChannelID:        session.ChannelID,
		MemberID:         session.MemberID,
		SessionID:        session.ID,
		HarnessKind:      session.Kind,
		NativeSessionRef: nativeSessionRef,
		TurnID:           turnID,
		StartedAt:        s.now(),
	})
	if err != nil {
		return harnessrun.Run{}, nil, nil, nil, err
	}
	if err := s.runs.Save(ctx, item); err != nil {
		return harnessrun.Run{}, nil, nil, nil, fmt.Errorf("persist harness run: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	steering := make(chan domain.PromptInput, 16)
	s.registerCancelHandle(item.ID, cancel)
	s.registerSteeringHandle(item.ID, steering)
	return item, runCtx, cancel, steering, nil
}

func runIDForConversationTurn(turnID string) string {
	value := strings.TrimSpace(turnID)
	value = strings.TrimPrefix(value, "turn-")
	value = strings.TrimPrefix(value, "conversation_")
	value = strings.TrimPrefix(value, "conversation-")
	return "run-" + value
}

func (s *Service) setRunNativeTurnID(ctx context.Context, item *harnessrun.Run, nativeTurnID string) error {
	if item == nil || strings.TrimSpace(nativeTurnID) == "" || strings.TrimSpace(item.NativeTurnID) == strings.TrimSpace(nativeTurnID) {
		return nil
	}
	if err := item.SetNativeTurnID(nativeTurnID); err != nil {
		return err
	}
	return s.runs.Save(ctx, *item)
}

func (s *Service) setRunNativeSessionRef(ctx context.Context, item *harnessrun.Run, nativeSessionRef string) error {
	if item == nil || strings.TrimSpace(nativeSessionRef) == "" || strings.TrimSpace(item.NativeSessionRef) == strings.TrimSpace(nativeSessionRef) {
		return nil
	}
	if err := item.SetNativeSessionRef(nativeSessionRef); err != nil {
		return err
	}
	return s.runs.Save(ctx, *item)
}

func (s *Service) markRunCanceled(ctx context.Context, item *harnessrun.Run) error {
	if item == nil {
		return nil
	}
	latest, err := s.runs.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("harness run %q not found", item.ID)
	}
	*item = *latest
	if err := item.MarkCanceled(s.now()); err != nil {
		return err
	}
	return s.runs.Save(ctx, *item)
}

func (s *Service) markRunCompleted(ctx context.Context, item *harnessrun.Run) error {
	if item == nil {
		return nil
	}
	latest, err := s.runs.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("harness run %q not found", item.ID)
	}
	*item = *latest
	if err := item.MarkCompleted(s.now()); err != nil {
		return err
	}
	return s.runs.Save(ctx, *item)
}

func (s *Service) markRunFailed(ctx context.Context, item *harnessrun.Run, runErr error) error {
	if item == nil || runErr == nil {
		return nil
	}
	latest, err := s.runs.Get(ctx, item.ID)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("harness run %q not found", item.ID)
	}
	*item = *latest
	if err := item.MarkFailed(runErr.Error(), s.now()); err != nil {
		return err
	}
	return s.runs.Save(ctx, *item)
}

func (s *Service) SendMessage(ctx context.Context, p SendMessageParams) (SendMessageResult, error) {
	spaceID := strings.TrimSpace(p.SpaceID)
	memberID := strings.TrimSpace(p.MemberID)
	text := strings.TrimSpace(p.Text)
	if spaceID == "" {
		return SendMessageResult{}, fmt.Errorf("spaceID is required")
	}
	if memberID == "" {
		return SendMessageResult{}, fmt.Errorf("memberID is required")
	}
	if strings.TrimSpace(p.ChannelID) == "" {
		return SendMessageResult{}, fmt.Errorf("channelID is required")
	}
	if strings.TrimSpace(p.ConversationMessageID) == "" {
		return SendMessageResult{}, fmt.Errorf("conversationMessageID is required")
	}
	localTurnID := "turn-" + strings.TrimSpace(p.ConversationMessageID)
	if text == "" && len(p.Attachments) == 0 {
		return SendMessageResult{}, fmt.Errorf("text or attachment is required")
	}
	session, err := s.repo.GetActiveByMember(ctx, memberID)
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("load active harness session for member %s: %w", memberID, err)
	}
	if session == nil {
		return SendMessageResult{}, fmt.Errorf("member %q has no active harness session", memberID)
	}
	if strings.TrimSpace(session.SpaceID) != spaceID {
		return SendMessageResult{}, fmt.Errorf("active harness session %q belongs to space %q, not %q", session.ID, session.SpaceID, spaceID)
	}
	if strings.TrimSpace(session.ChannelID) != strings.TrimSpace(p.ChannelID) {
		return SendMessageResult{}, fmt.Errorf("active harness session %q belongs to channel %q, not %q", session.ID, session.ChannelID, p.ChannelID)
	}
	if strings.TrimSpace(session.SystemPrompt) == "" {
		return SendMessageResult{}, fmt.Errorf("active harness session %q has no provisioned system prompt", session.ID)
	}
	sessionRef := strings.TrimSpace(session.Ref)
	runtime, err := s.registry.Get(session.Kind)
	if err != nil {
		return SendMessageResult{}, err
	}
	sessionRuntime, ok := runtime.(domain.SessionRuntime)
	if !ok {
		return SendMessageResult{}, fmt.Errorf("harness %q does not support chat session turns", session.Kind)
	}
	s.logger.InfoContext(ctx, "harness session turn starting",
		"session_id", session.ID,
		"space_id", session.SpaceID,
		"member_id", session.MemberID,
		"channel_id", session.ChannelID,
		"harness_kind", session.Kind,
		"location_id", session.LocationID,
		"has_session_ref", strings.TrimSpace(session.Ref) != "",
		"mcp_server_count", len(compactStrings(session.MCPServers)),
	)

	turnID := localTurnID
	var response strings.Builder
	var sendErr error
	streamSequence := 0
	params := domain.StartParams{
		Workdir:         strings.TrimSpace(session.Workdir),
		Model:           strings.TrimSpace(session.Model),
		ReasoningEffort: strings.TrimSpace(session.Effort),
		SystemPrompt:    strings.TrimSpace(session.SystemPrompt),
		MCPServers:      append([]string(nil), session.MCPServers...),
		PermissionMode:  strings.TrimSpace(session.PermissionMode),
		ConfigRef:       strings.TrimSpace(session.ConfigRef),
		SessionRef:      sessionRef,
		Continue:        sessionRef != "",
		PersistSessionRef: func(nextRef string) error {
			nextRef = strings.TrimSpace(nextRef)
			if nextRef == "" || nextRef == session.Ref {
				return nil
			}
			session.Ref = nextRef
			return s.repo.Save(ctx, session)
		},
	}
	if s.humanInputAwaiter != nil {
		params.ApprovalHandler = s.approvalHandlerForSession(session)
	}
	attachments := append([]PromptAttachment(nil), p.Attachments...)
	if s.attachmentStager != nil {
		attachments, err = s.attachmentStager.StageAttachments(ctx, AttachmentStageRequest{
			LocationID:  strings.TrimSpace(session.LocationID),
			Workdir:     strings.TrimSpace(session.Workdir),
			Attachments: attachments,
		})
		if err != nil {
			return SendMessageResult{}, fmt.Errorf("stage harness attachments: %w", err)
		}
	}
	if p.AllowSteering {
		result, steered, steerErr := s.steerActiveRun(ctx, session, runtime, p, attachments)
		if steerErr != nil {
			return SendMessageResult{}, steerErr
		}
		if steered {
			return result, nil
		}
	}
	if p.SteerOnly {
		return SendMessageResult{}, fmt.Errorf("%s for session %q", errNoActiveHarnessRunToSteer, session.ID)
	}
	if s.runtimeHosts != nil && strings.TrimSpace(session.LocationID) != "" {
		s.logger.InfoContext(ctx, "harness resolving runtime host",
			"session_id", session.ID,
			"member_id", session.MemberID,
			"harness_kind", session.Kind,
			"location_id", session.LocationID,
		)
		host, err := s.runtimeHosts.ResolveRuntimeHost(ctx, RuntimeHostRequest{
			LocationID:  strings.TrimSpace(session.LocationID),
			HarnessKind: strings.TrimSpace(session.Kind),
			SessionID:   strings.TrimSpace(session.ID),
			ProjectID:   strings.TrimSpace(session.ProjectID),
			MemberID:    strings.TrimSpace(session.MemberID),
			SessionRef:  strings.TrimSpace(session.Ref),
			MCPToken:    strings.TrimSpace(session.MCPToken),
		})
		if err != nil {
			s.logger.ErrorContext(ctx, "harness runtime host resolution failed",
				"session_id", session.ID,
				"member_id", session.MemberID,
				"harness_kind", session.Kind,
				"location_id", session.LocationID,
				"error", err,
			)
			return SendMessageResult{}, err
		}
		params.AppServerURL = strings.TrimSpace(host.AppServerURL)
		params.RuntimeHostDiagnostics = strings.TrimSpace(host.Diagnostics)
		s.logger.InfoContext(ctx, "harness runtime host resolved",
			"session_id", session.ID,
			"member_id", session.MemberID,
			"harness_kind", session.Kind,
			"location_id", session.LocationID,
			"has_app_server_url", params.AppServerURL != "",
			"has_mcp_base_url", strings.TrimSpace(host.MCPBaseURL) != "",
			"diagnostics", params.RuntimeHostDiagnostics,
		)
		if mcpBaseURL := strings.TrimSpace(host.MCPBaseURL); mcpBaseURL != "" {
			params.MCPServers, err = s.formatMCPServers(ctx, MCPConfigRequest{
				HarnessKind: session.Kind,
				BaseURL:     mcpBaseURL,
				Token:       session.MCPToken,
			})
			if err != nil {
				s.logger.ErrorContext(ctx, "harness mcp config format failed",
					"session_id", session.ID,
					"member_id", session.MemberID,
					"harness_kind", session.Kind,
					"location_id", session.LocationID,
					"error", err,
				)
				return SendMessageResult{}, err
			}
			s.logger.InfoContext(ctx, "harness mcp config formatted",
				"session_id", session.ID,
				"member_id", session.MemberID,
				"harness_kind", session.Kind,
				"location_id", session.LocationID,
				"mcp_server_count", len(compactStrings(params.MCPServers)),
			)
		}
	}
	runItem, runCtx, cancel, steering, err := s.startRun(ctx, session, localTurnID, sessionRef)
	if err != nil {
		return SendMessageResult{}, err
	}
	defer s.clearCancelHandle(runItem.ID)
	defer s.clearSteeringHandle(runItem.ID)
	defer cancel()
	streamSequence++
	if err := emitRunActivity(ctx, p.OnActivity, session.ID, localTurnID, runItem, streamSequence, "harness.run.started", "running", "Run started", ""); err != nil {
		return SendMessageResult{}, err
	}
	_, err = sessionRuntime.ExecuteSessionTurn(runCtx, params, domain.SessionTurnInput{TurnID: localTurnID, Text: text, Attachments: domainAttachmentsFromApp(attachments), Steering: steering}, func(ev domain.Event) {
		if ev.TurnID != "" {
			turnID = ev.TurnID
			if ev.TurnID != localTurnID {
				if updateErr := s.setRunNativeTurnID(ctx, &runItem, ev.TurnID); updateErr != nil && sendErr == nil {
					sendErr = updateErr
				}
			}
		}
		if ev.SessionRef != "" {
			sessionRef = ev.SessionRef
			if updateErr := s.setRunNativeSessionRef(ctx, &runItem, ev.SessionRef); updateErr != nil && sendErr == nil {
				sendErr = updateErr
			}
		}
		if ev.Type == domain.EventText && isAssistantConversationText(ev) {
			streamSequence++
			response.WriteString(ev.Text)
			if p.OnAssistantDelta != nil {
				if err := p.OnAssistantDelta(ctx, AssistantDelta{
					SessionID: session.ID,
					TurnID:    turnID,
					Sequence:  streamSequence,
					Text:      ev.Text,
				}); err != nil {
					sendErr = err
				}
			}
		}
		if ev.Type == domain.EventText && isReasoningConversationText(ev) {
			streamSequence++
			if p.OnThinkingDelta != nil {
				if err := p.OnThinkingDelta(ctx, ThinkingDelta{
					SessionID: session.ID,
					TurnID:    turnID,
					Sequence:  streamSequence,
					Text:      ev.Text,
					Data:      ev.Data,
				}); err != nil {
					sendErr = err
				}
			}
		}
		if isToolActivityEvent(ev) && p.OnActivity != nil {
			streamSequence++
			if err := p.OnActivity(ctx, activityEventFromRuntime(session.ID, turnID, streamSequence, ev)); err != nil {
				sendErr = err
			}
		}
	})
	if sendErr != nil {
		s.logger.ErrorContext(ctx, "harness session turn stream failed",
			"session_id", session.ID,
			"member_id", session.MemberID,
			"harness_kind", session.Kind,
			"location_id", session.LocationID,
			"turn_id", turnID,
			"error", sendErr,
		)
		return SendMessageResult{}, fmt.Errorf("stream assistant response for harness session %s: %w", session.ID, sendErr)
	}
	if err != nil {
		if shouldResetRuntimeSessionRefAfterFailure(session.Kind, err) {
			session.Ref = ""
			if saveErr := s.repo.Save(ctx, session); saveErr != nil {
				return SendMessageResult{}, fmt.Errorf("reset failed harness session ref: %w", saveErr)
			}
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, domain.ErrTurnCanceled) {
			if markErr := s.markRunCanceled(ctx, &runItem); markErr != nil {
				return SendMessageResult{}, markErr
			}
			streamSequence++
			if activityErr := emitRunActivity(ctx, p.OnActivity, session.ID, turnID, runItem, streamSequence, "harness.run.canceled", "canceled", "Run stopped", ""); activityErr != nil {
				return SendMessageResult{}, activityErr
			}
			return SendMessageResult{
				SessionID: session.ID,
				RunID:     runItem.ID,
				TurnID:    turnID,
				Delivery:  "delivered",
				Text:      strings.TrimSpace(response.String()),
			}, nil
		}
		if markErr := s.markRunFailed(ctx, &runItem, err); markErr != nil {
			return SendMessageResult{}, markErr
		}
		streamSequence++
		if activityErr := emitRunActivity(ctx, p.OnActivity, session.ID, turnID, runItem, streamSequence, "harness.run.failed", "failed", "Run failed", err.Error()); activityErr != nil {
			return SendMessageResult{}, activityErr
		}
		s.logger.ErrorContext(ctx, "harness session turn failed",
			"session_id", session.ID,
			"member_id", session.MemberID,
			"harness_kind", session.Kind,
			"location_id", session.LocationID,
			"turn_id", turnID,
			"has_session_ref", sessionRef != "",
			"error", err,
		)
		return SendMessageResult{}, fmt.Errorf("send message to harness session %s: %w", session.ID, err)
	}
	if err := s.markRunCompleted(ctx, &runItem); err != nil {
		return SendMessageResult{}, err
	}
	streamSequence++
	if err := emitRunActivity(ctx, p.OnActivity, session.ID, turnID, runItem, streamSequence, "harness.run.completed", "completed", "Run completed", ""); err != nil {
		return SendMessageResult{}, err
	}
	if sessionRef != "" && session.Ref != sessionRef {
		session.Ref = sessionRef
		if err := s.repo.Save(ctx, session); err != nil {
			return SendMessageResult{}, fmt.Errorf("persist harness session ref: %w", err)
		}
	}
	s.logger.InfoContext(ctx, "harness session turn delivered",
		"session_id", session.ID,
		"member_id", session.MemberID,
		"harness_kind", session.Kind,
		"location_id", session.LocationID,
		"turn_id", turnID,
		"has_session_ref", sessionRef != "",
		"stream_events", streamSequence,
	)
	return SendMessageResult{
		SessionID: session.ID,
		RunID:     runItem.ID,
		TurnID:    turnID,
		Delivery:  "delivered",
		Text:      strings.TrimSpace(response.String()),
	}, nil
}

func shouldResetRuntimeSessionRefAfterFailure(kind string, err error) bool {
	if err == nil || domain.NormalizeRuntimeKind(kind) != "claude-cli" {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "`thinking` or `redacted_thinking` blocks") &&
		strings.Contains(text, "latest assistant message cannot be modified")
}

func (s *Service) steerActiveRun(ctx context.Context, session *domain.Session, runtime domain.Runtime, p SendMessageParams, attachments []PromptAttachment) (SendMessageResult, bool, error) {
	active, err := s.runs.GetActiveBySession(ctx, session.ID)
	if err != nil {
		return SendMessageResult{}, false, err
	}
	if active == nil || s.cancelHandle(active.ID) == nil {
		return SendMessageResult{}, false, nil
	}
	if !runtimeSupportsSessionSteering(runtime) {
		return SendMessageResult{}, true, fmt.Errorf("harness session %q already has active run %q", session.ID, active.ID)
	}
	steering := s.steeringHandle(active.ID)
	if steering == nil {
		return SendMessageResult{}, true, fmt.Errorf("harness run %q is active but has no steering handle", active.ID)
	}
	input := domain.PromptInput{
		TurnID:          active.TurnID,
		Text:            strings.TrimSpace(p.Text),
		Attachments:     domainAttachmentsFromApp(attachments),
		ReasoningEffort: strings.TrimSpace(session.Effort),
	}
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return SendMessageResult{}, true, fmt.Errorf("text or attachment is required")
	}
	select {
	case steering <- input:
		s.logger.InfoContext(ctx, "harness session turn steered",
			"session_id", session.ID,
			"space_id", session.SpaceID,
			"member_id", session.MemberID,
			"channel_id", session.ChannelID,
			"harness_kind", session.Kind,
			"run_id", active.ID,
			"turn_id", active.TurnID,
			"conversation_message_id", strings.TrimSpace(p.ConversationMessageID),
		)
		return SendMessageResult{
			SessionID: session.ID,
			RunID:     active.ID,
			TurnID:    active.TurnID,
			Delivery:  "steered",
		}, true, nil
	case <-ctx.Done():
		return SendMessageResult{}, true, ctx.Err()
	default:
		return SendMessageResult{}, true, fmt.Errorf("harness run %q steering queue is full", active.ID)
	}
}

func runtimeSupportsSessionSteering(runtime domain.Runtime) bool {
	steeringRuntime, ok := runtime.(domain.SessionSteeringRuntime)
	return ok && steeringRuntime.SupportsSessionSteering()
}

func (s *Service) startParamsForSession(ctx context.Context, session *domain.Session) (domain.StartParams, error) {
	if session == nil {
		return domain.StartParams{}, fmt.Errorf("session is required")
	}
	params := domain.StartParams{
		Workdir:         strings.TrimSpace(session.Workdir),
		Model:           strings.TrimSpace(session.Model),
		ReasoningEffort: strings.TrimSpace(session.Effort),
		SystemPrompt:    strings.TrimSpace(session.SystemPrompt),
		MCPServers:      append([]string(nil), session.MCPServers...),
		PermissionMode:  strings.TrimSpace(session.PermissionMode),
		ConfigRef:       strings.TrimSpace(session.ConfigRef),
		SessionRef:      strings.TrimSpace(session.Ref),
		Continue:        strings.TrimSpace(session.Ref) != "",
	}
	if s.runtimeHosts != nil && strings.TrimSpace(session.LocationID) != "" {
		host, err := s.runtimeHosts.ResolveRuntimeHost(ctx, RuntimeHostRequest{
			LocationID:  strings.TrimSpace(session.LocationID),
			HarnessKind: strings.TrimSpace(session.Kind),
			SessionID:   strings.TrimSpace(session.ID),
			ProjectID:   strings.TrimSpace(session.ProjectID),
			MemberID:    strings.TrimSpace(session.MemberID),
			SessionRef:  strings.TrimSpace(session.Ref),
			MCPToken:    strings.TrimSpace(session.MCPToken),
		})
		if err != nil {
			return domain.StartParams{}, err
		}
		params.AppServerURL = strings.TrimSpace(host.AppServerURL)
		params.RuntimeHostDiagnostics = strings.TrimSpace(host.Diagnostics)
		if mcpBaseURL := strings.TrimSpace(host.MCPBaseURL); mcpBaseURL != "" {
			servers, err := s.formatMCPServers(ctx, MCPConfigRequest{
				HarnessKind: session.Kind,
				BaseURL:     mcpBaseURL,
				Token:       session.MCPToken,
			})
			if err != nil {
				return domain.StartParams{}, err
			}
			params.MCPServers = servers
		}
	}
	return params, nil
}

func isAssistantConversationText(ev domain.Event) bool {
	kind := strings.TrimSpace(ev.Data["kind"])
	return kind == "" || kind == "assistant"
}

func isUserConversationText(ev domain.Event) bool {
	return strings.TrimSpace(ev.Data["kind"]) == "user"
}

func isReasoningConversationText(ev domain.Event) bool {
	return strings.TrimSpace(ev.Data["kind"]) == "reasoning"
}

func domainAttachmentsFromApp(attachments []PromptAttachment) []domain.PromptAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]domain.PromptAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, domain.PromptAttachment{
			ID:        attachment.ID,
			Name:      attachment.Name,
			MediaType: attachment.MediaType,
			SizeBytes: attachment.SizeBytes,
			URI:       attachment.URI,
		})
	}
	return out
}

func isToolActivityEvent(ev domain.Event) bool {
	return ev.Type == domain.EventToolCall || ev.Type == domain.EventToolResult
}

func (s *Service) approvalHandlerForSession(session *domain.Session) domain.ApprovalHandler {
	return func(ctx context.Context, req domain.ApprovalRequest) (domain.ApprovalDecision, error) {
		if s == nil || s.humanInputAwaiter == nil {
			return domain.ApprovalDecision{}, fmt.Errorf("human input awaiter is required for harness approval")
		}
		toolCallID := strings.TrimSpace(req.ToolCallID)
		if toolCallID == "" {
			toolCallID = strings.TrimSpace(req.ApprovalID)
		}
		if toolCallID == "" {
			return domain.ApprovalDecision{}, fmt.Errorf("approval tool call id is required")
		}
		toolName := strings.TrimSpace(req.ToolName)
		if toolName == "" {
			toolName = "runtime_approval"
		}
		payload := humaninputdomain.ApproveRejectPayload{
			Title:       approvalTitle(req),
			Description: approvalDescription(req),
			Context:     approvalContextForSession(session, req),
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return domain.ApprovalDecision{}, fmt.Errorf("encode approval payload: %w", err)
		}
		resultBytes, err := s.humanInputAwaiter.Await(ctx, humaninputdomain.PendingRequest{
			ToolCallID:     toolCallID,
			ToolName:       toolName,
			IdempotencyKey: approvalIdempotencyKey(req, toolCallID),
			ProjectID:      session.ProjectID,
			SpaceID:        session.SpaceID,
			MemberID:       session.MemberID,
			ChannelID:      session.ChannelID,
			Declaration: humaninputdomain.Declaration{
				Kind:    humaninputdomain.PrimitiveApproveReject,
				Payload: payloadBytes,
			},
		})
		if err != nil {
			return domain.ApprovalDecision{}, err
		}
		var result humaninputdomain.ApproveRejectResult
		if err := json.Unmarshal(resultBytes, &result); err != nil {
			return domain.ApprovalDecision{}, fmt.Errorf("decode approval result: %w", err)
		}
		if result.Cancelled {
			return domain.ApprovalDecision{Decision: "reject", Note: "cancelled"}, nil
		}
		decision := strings.ToLower(strings.TrimSpace(result.Decision))
		switch decision {
		case "approve", "reject":
			return domain.ApprovalDecision{Decision: decision, Note: strings.TrimSpace(result.Note)}, nil
		default:
			return domain.ApprovalDecision{}, fmt.Errorf("unsupported approval decision %q", result.Decision)
		}
	}
}

func approvalTitle(req domain.ApprovalRequest) string {
	if summary := strings.TrimSpace(req.Summary); summary != "" {
		return summary
	}
	if command := strings.TrimSpace(req.Command); command != "" {
		return "Approve command execution"
	}
	if path := strings.TrimSpace(req.Path); path != "" {
		return "Approve file change for " + path
	}
	return "Approve runtime permission request"
}

func approvalDescription(req domain.ApprovalRequest) string {
	if command := strings.TrimSpace(req.Command); command != "" {
		return command
	}
	if path := strings.TrimSpace(req.Path); path != "" {
		return path
	}
	return strings.TrimSpace(req.Method)
}

func approvalContext(req domain.ApprovalRequest) string {
	pairs := make([]string, 0, len(req.Data)+2)
	if method := strings.TrimSpace(req.Method); method != "" {
		pairs = append(pairs, "method="+method)
	}
	if approvalID := strings.TrimSpace(req.ApprovalID); approvalID != "" {
		pairs = append(pairs, "approvalId="+approvalID)
	}
	for key, value := range req.Data {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	return strings.Join(pairs, "\n")
}

func approvalContextForSession(session *domain.Session, req domain.ApprovalRequest) string {
	if session == nil {
		return approvalContext(req)
	}
	data := make(map[string]string, len(req.Data)+6)
	for key, value := range req.Data {
		data[key] = value
	}
	data["harness"] = session.Kind
	data["memberId"] = session.MemberID
	data["spaceId"] = session.SpaceID
	data["channelId"] = session.ChannelID
	data["locationId"] = session.LocationID
	if strings.TrimSpace(session.Workdir) != "" {
		data["workdir"] = session.Workdir
	}
	withSession := req
	withSession.Data = data
	return approvalContext(withSession)
}

func approvalIdempotencyKey(req domain.ApprovalRequest, toolCallID string) string {
	if approvalID := strings.TrimSpace(req.ApprovalID); approvalID != "" {
		return strings.TrimSpace(req.Method + ":" + approvalID)
	}
	return strings.TrimSpace(req.Method + ":" + toolCallID)
}

func activityEventFromRuntime(sessionID, turnID string, sequence int, ev domain.Event) ActivityEvent {
	data := make(map[string]string, len(ev.Data)+1)
	for k, v := range ev.Data {
		if strings.TrimSpace(k) != "" {
			data[k] = v
		}
	}
	status := strings.TrimSpace(data["status"])
	if status == "" {
		if ev.Type == domain.EventToolResult {
			status = "completed"
		} else {
			status = "pending"
		}
	}
	if ev.Error != "" {
		data["error"] = strings.TrimSpace(ev.Error)
		status = "failed"
	}
	if deniedActivity(data, ev.Text) {
		status = "failed"
		if strings.TrimSpace(data["error"]) == "" {
			data["error"] = firstNonEmpty(data["result"], data["outputPreview"], ev.Text, "operation denied")
		}
	}
	if strings.TrimSpace(data["toolName"]) == "" && strings.TrimSpace(ev.ToolName) != "" {
		data["toolName"] = strings.TrimSpace(ev.ToolName)
	}
	if strings.TrimSpace(data["outputPreview"]) == "" && strings.TrimSpace(ev.Text) != "" {
		data["outputPreview"] = strings.TrimSpace(ev.Text)
	}
	return ActivityEvent{
		SessionID:  sessionID,
		TurnID:     firstNonEmpty(ev.TurnID, turnID),
		ToolCallID: strings.TrimSpace(ev.ToolCallID),
		ToolName:   strings.TrimSpace(ev.ToolName),
		Sequence:   sequence,
		Status:     status,
		Text:       strings.TrimSpace(ev.Text),
		Data:       data,
	}
}

func emitRunActivity(ctx context.Context, emit func(context.Context, ActivityEvent) error, sessionID, turnID string, item harnessrun.Run, sequence int, kind, status, text, errText string) error {
	if emit == nil {
		return nil
	}
	data := map[string]string{
		"kind":             kind,
		"runId":            item.ID,
		"sessionId":        item.SessionID,
		"turnId":           firstNonEmpty(turnID, item.NativeTurnID, item.TurnID),
		"localTurnId":      item.TurnID,
		"nativeTurnId":     item.NativeTurnID,
		"nativeSessionRef": item.NativeSessionRef,
		"harnessKind":      item.HarnessKind,
		"status":           status,
	}
	if errText = strings.TrimSpace(errText); errText != "" {
		data["error"] = errText
	}
	return emit(ctx, ActivityEvent{
		SessionID:  sessionID,
		TurnID:     firstNonEmpty(turnID, item.NativeTurnID, item.TurnID),
		ToolCallID: item.ID + ":" + kind,
		ToolName:   kind,
		Sequence:   sequence,
		Status:     status,
		Text:       strings.TrimSpace(text),
		Data:       data,
	})
}

func deniedActivity(data map[string]string, text string) bool {
	status := strings.ToLower(strings.TrimSpace(data["status"]))
	switch status {
	case "denied", "rejected", "cancelled", "canceled", "permission_denied", "approval_denied":
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		data["error"],
		data["result"],
		data["outputPreview"],
		data["stderr"],
		text,
	}, "\n"))
	return strings.Contains(haystack, "permission denied") ||
		strings.Contains(haystack, "approval denied") ||
		strings.Contains(haystack, "was denied") ||
		strings.Contains(haystack, "denied by")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *Service) ActivateSession(ctx context.Context, p ActivateSessionParams) (*domain.Session, error) {
	runtimeContext, err := s.runtimeContextFromActivation(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateRuntimeConfig(runtimeContext.HarnessKind, runtimeContext.Model, runtimeContext.Effort, runtimeContext.PermissionMode, runtimeContext.ConfigRef); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetActiveByMember(ctx, runtimeContext.MemberID)
	if err != nil {
		return nil, fmt.Errorf("check existing session: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("member %q already has an active session %q", runtimeContext.MemberID, existing.ID)
	}

	session, err := domain.NewSession(s.newID(), runtimeContext, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness session activation failed", "operation", "activate_session", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "error", err)
		return nil, fmt.Errorf("persist session: %w", err)
	}
	s.logger.InfoContext(ctx, "harness session activated", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "harness_kind", session.Kind, "model", session.Model, "effort", session.Effort)
	return session, nil
}

func (s *Service) runtimeContextFromActivation(ctx context.Context, p ActivateSessionParams) (domain.RuntimeContext, error) {
	if s.mcpProvisioner != nil && len(compactStrings(p.MCPServers)) == 0 {
		provisioned, err := s.mcpProvisioner.ProvisionMCP(ctx, p)
		if err != nil {
			return domain.RuntimeContext{}, fmt.Errorf("provision mcp runtime context: %w", err)
		}
		p.MCPToken = provisioned.Token
		p.MCPServers, err = s.formatMCPServers(ctx, MCPConfigRequest{
			HarnessKind: p.HarnessKind,
			RawURL:      provisioned.URL,
		})
		if err != nil {
			return domain.RuntimeContext{}, fmt.Errorf("format mcp runtime context: %w", err)
		}
	}
	runtimeCtx := domain.RuntimeContext{
		ProjectID:      strings.TrimSpace(p.ProjectID),
		LocationID:     "local",
		MemberID:       strings.TrimSpace(p.MemberID),
		SpaceID:        strings.TrimSpace(p.SpaceID),
		ChannelID:      strings.TrimSpace(p.ChannelID),
		DisplayName:    strings.TrimSpace(p.DisplayName),
		MemberType:     strings.TrimSpace(p.MemberType),
		LifecycleState: strings.TrimSpace(p.LifecycleState),
		HarnessKind:    strings.TrimSpace(p.HarnessKind),
		Model:          strings.TrimSpace(p.Model),
		Effort:         strings.TrimSpace(p.Effort),
		PermissionMode: strings.TrimSpace(p.PermissionMode),
		ConfigRef:      strings.TrimSpace(p.ConfigRef),
		SessionRef:     strings.TrimSpace(p.SessionRef),
		MCPToken:       strings.TrimSpace(p.MCPToken),
		MCPServers:     compactStrings(p.MCPServers),
	}
	if runtimeCtx.PermissionMode == "" {
		runtimeCtx.PermissionMode = s.catalog.DefaultPermissionMode(runtimeCtx.HarnessKind)
	}
	workdir, err := s.resolveHarnessWorkdir(ctx, runtimeCtx.ProjectID)
	if err != nil {
		return domain.RuntimeContext{}, err
	}
	runtimeCtx.LocationID = workdir.LocationID
	runtimeCtx.Workdir = workdir.Workdir
	prompt, err := ManagedMemberSystemPrompt(MemberPromptInput{
		MemberID:       runtimeCtx.MemberID,
		SpaceID:        runtimeCtx.SpaceID,
		ChannelID:      runtimeCtx.ChannelID,
		DisplayName:    runtimeCtx.DisplayName,
		MemberType:     runtimeCtx.MemberType,
		LifecycleState: runtimeCtx.LifecycleState,
		Kind:           runtimeCtx.HarnessKind,
		Model:          runtimeCtx.Model,
	})
	if err != nil {
		return domain.RuntimeContext{}, err
	}
	runtimeCtx.SystemPrompt = prompt
	return runtimeCtx, nil
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (s *Service) DeactivateSession(ctx context.Context, id string, reason domain.InactiveReason, errDetail string) error {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session %q not found", id)
	}
	if err := session.Deactivate(reason, errDetail, s.now()); err != nil {
		return err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness session deactivation failed", "operation", "deactivate_session", "session_id", id, "error", err)
		return fmt.Errorf("persist session: %w", err)
	}
	s.logger.InfoContext(ctx, "harness session deactivated", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "reason", session.InactiveReason)
	return nil
}

func (s *Service) ReactivateSession(ctx context.Context, id string) (*domain.Session, error) {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}

	existing, err := s.repo.GetActiveByMember(ctx, session.MemberID)
	if err != nil {
		return nil, fmt.Errorf("check existing session: %w", err)
	}
	if existing != nil && existing.ID != id {
		return nil, fmt.Errorf("member %q already has an active session %q", session.MemberID, existing.ID)
	}

	if err := session.Reactivate(s.now()); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness session reactivation failed", "operation", "reactivate_session", "session_id", id, "error", err)
		return nil, fmt.Errorf("persist session: %w", err)
	}
	s.logger.InfoContext(ctx, "harness session reactivated", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "harness_kind", session.Kind, "model", session.Model, "effort", session.Effort)
	return session, nil
}

func (s *Service) UpdateSessionConfig(ctx context.Context, id, model, effort string) (*domain.Session, error) {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	permissionMode := strings.TrimSpace(session.PermissionMode)
	if permissionMode == "" {
		permissionMode = s.catalog.DefaultPermissionMode(session.Kind)
	}
	if err := s.ValidateRuntimeConfig(session.Kind, model, effort, permissionMode, session.ConfigRef); err != nil {
		return nil, err
	}
	if err := session.UpdateRuntimeConfigValues(model, effort, permissionMode, session.ConfigRef); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness session config update failed", "operation", "update_session_config", "session_id", id, "error", err)
		return nil, fmt.Errorf("persist session: %w", err)
	}
	if err := s.invalidateRuntimeSessionRef(session); err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "harness session config updated", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "harness_kind", session.Kind, "model", session.Model, "effort", session.Effort)
	return session, nil
}

func (s *Service) UpdateSessionRuntimeContext(ctx context.Context, id string, p ActivateSessionParams) (*domain.Session, error) {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %q not found", id)
	}
	runtimeContext, err := s.runtimeContextFromActivation(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateRuntimeConfig(runtimeContext.HarnessKind, runtimeContext.Model, runtimeContext.Effort, runtimeContext.PermissionMode, runtimeContext.ConfigRef); err != nil {
		return nil, err
	}
	if err := session.UpdateRuntimeContext(runtimeContext); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness session runtime context update failed", "operation", "update_session_runtime_context", "session_id", id, "error", err)
		return nil, fmt.Errorf("persist session: %w", err)
	}
	if err := s.invalidateRuntimeSessionRef(session); err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "harness session runtime context updated", "session_id", session.ID, "space_id", session.SpaceID, "member_id", session.MemberID, "harness_kind", session.Kind, "model", session.Model, "effort", session.Effort)
	return session, nil
}

func (s *Service) invalidateRuntimeSessionRef(session *domain.Session) error {
	if session == nil {
		return fmt.Errorf("session is required")
	}
	sessionRef := strings.TrimSpace(session.Ref)
	if sessionRef == "" {
		return nil
	}
	runtime, err := s.registry.Get(session.Kind)
	if err != nil {
		return err
	}
	invalidator, ok := runtime.(domain.SessionRuntimeInvalidator)
	if !ok {
		return nil
	}
	if err := invalidator.InvalidateSessionRef(sessionRef); err != nil {
		return fmt.Errorf("invalidate harness runtime session %s: %w", session.ID, err)
	}
	s.logger.Info("harness runtime session invalidated", "session_id", session.ID, "session_ref", sessionRef, "harness_kind", session.Kind)
	return nil
}

func (s *Service) RefreshSessionMCPConfig(ctx context.Context, id string, servers []string) (*domain.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	servers = compactStrings(servers)
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp servers are required")
	}
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load harness session %s: %w", id, err)
	}
	if session == nil {
		return nil, fmt.Errorf("harness session %s not found", id)
	}
	if session.Status != domain.SessionActive {
		return nil, fmt.Errorf("cannot refresh session %q mcp config: status is %q, not active", session.ID, session.Status)
	}
	session.MCPServers = append([]string(nil), servers...)
	if err := s.repo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save harness session %s mcp config: %w", session.ID, err)
	}
	return session, nil
}

func (s *Service) RefreshSessionMCPURL(ctx context.Context, id string, rawURL string) (*domain.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load harness session %s: %w", id, err)
	}
	if session == nil {
		return nil, fmt.Errorf("harness session %s not found", id)
	}
	servers, err := s.formatMCPServers(ctx, MCPConfigRequest{
		HarnessKind: session.Kind,
		RawURL:      rawURL,
	})
	if err != nil {
		return nil, err
	}
	if equalStringSlices(session.MCPServers, servers) {
		return session, nil
	}
	return s.RefreshSessionMCPConfig(ctx, id, servers)
}

func (s *Service) RefreshSessionMCPBinding(ctx context.Context, id string, token string, rawURL string) (*domain.Session, error) {
	id = strings.TrimSpace(id)
	token = strings.TrimSpace(token)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if token == "" {
		return nil, fmt.Errorf("mcp token is required")
	}
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load harness session %s: %w", id, err)
	}
	if session == nil {
		return nil, fmt.Errorf("harness session %s not found", id)
	}
	if session.Status != domain.SessionActive {
		return nil, fmt.Errorf("cannot refresh session %q mcp binding: status is %q, not active", session.ID, session.Status)
	}
	servers, err := s.formatMCPServers(ctx, MCPConfigRequest{
		HarnessKind: session.Kind,
		RawURL:      rawURL,
	})
	if err != nil {
		return nil, err
	}
	if session.MCPToken == token && equalStringSlices(session.MCPServers, servers) {
		return session, nil
	}
	session.MCPToken = token
	session.MCPServers = append([]string(nil), servers...)
	if err := s.repo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save harness session %s mcp binding: %w", session.ID, err)
	}
	return session, nil
}

func (s *Service) RefreshSessionClaudeChannelURL(ctx context.Context, id string, rawURL string) (*domain.Session, error) {
	return s.RefreshSessionClaudeChannelRoute(ctx, id, rawURL, "legacy-"+rawURL, time.Now().UTC())
}

func (s *Service) RefreshSessionClaudeChannelRoute(ctx context.Context, id string, rawURL string, instanceID string, startedAt time.Time) (*domain.Session, error) {
	id = strings.TrimSpace(id)
	rawURL = strings.TrimSpace(rawURL)
	instanceID = strings.TrimSpace(instanceID)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if rawURL == "" {
		return nil, fmt.Errorf("claude channel url is required")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("claude channel instance id is required")
	}
	if startedAt.IsZero() {
		return nil, fmt.Errorf("claude channel started at is required")
	}
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load harness session %s: %w", id, err)
	}
	if session == nil {
		return nil, fmt.Errorf("harness session %s not found", id)
	}
	if !strings.EqualFold(strings.TrimSpace(session.Kind), "claude-cli") {
		return nil, fmt.Errorf("harness session %s is %q, not claude-cli", session.ID, session.Kind)
	}
	if session.ClaudeChannelURL == rawURL &&
		session.ClaudeChannelInstanceID == instanceID &&
		session.ClaudeChannelStartedAt != nil &&
		session.ClaudeChannelStartedAt.Equal(startedAt.UTC()) {
		return session, nil
	}
	if err := session.UpdateClaudeChannelRoute(rawURL, instanceID, startedAt); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save harness session %s claude channel url: %w", session.ID, err)
	}
	return session, nil
}

func (s *Service) RefreshSessionWorkdir(ctx context.Context, id string) (*domain.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id is required")
	}
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load harness session %s: %w", id, err)
	}
	if session == nil {
		return nil, fmt.Errorf("harness session %s not found", id)
	}
	if session.Status != domain.SessionActive {
		return nil, fmt.Errorf("cannot refresh session %q workdir: status is %q, not active", session.ID, session.Status)
	}
	workdir, err := s.resolveHarnessWorkdir(ctx, session.ProjectID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(session.Workdir) == workdir.Workdir && strings.TrimSpace(session.LocationID) == workdir.LocationID {
		return session, nil
	}
	session.LocationID = workdir.LocationID
	session.Workdir = workdir.Workdir
	session.Ref = ""
	if err := s.repo.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("save harness session %s workdir: %w", session.ID, err)
	}
	return session, nil
}

func (s *Service) resolveHarnessWorkdir(ctx context.Context, projectID string) (ProjectWorkdir, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectWorkdir{}, fmt.Errorf("projectID is required")
	}
	if s.workdirResolver == nil {
		return ProjectWorkdir{}, fmt.Errorf("project workdir resolver is required")
	}
	workdir, err := s.workdirResolver.ResolveHarnessWorkdir(ctx, projectID)
	if err != nil {
		return ProjectWorkdir{}, err
	}
	workdir.LocationID = strings.TrimSpace(workdir.LocationID)
	if workdir.LocationID == "" {
		return ProjectWorkdir{}, fmt.Errorf("project location is required")
	}
	workdir.Workdir = strings.TrimSpace(workdir.Workdir)
	if workdir.Workdir == "" {
		return ProjectWorkdir{}, fmt.Errorf("project workdir is required")
	}
	return workdir, nil
}

func (s *Service) formatMCPServers(ctx context.Context, request MCPConfigRequest) ([]string, error) {
	if s.mcpConfigFormatter == nil {
		return nil, fmt.Errorf("mcp config formatter is required")
	}
	servers, err := s.mcpConfigFormatter.FormatMCPServers(ctx, request)
	if err != nil {
		return nil, err
	}
	servers = compactStrings(servers)
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp servers are required")
	}
	return servers, nil
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Service) RecordUsage(ctx context.Context, id string, tokensIn, tokensOut int64) error {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session %q not found", id)
	}
	session.AddUsage(tokensIn, tokensOut)
	if err := s.repo.Save(ctx, session); err != nil {
		s.logger.ErrorContext(ctx, "harness usage accounting failed", "operation", "record_usage", "session_id", id, "error", err)
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}

func (s *Service) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) GetActiveSession(ctx context.Context, memberID string) (*domain.Session, error) {
	return s.repo.GetActiveByMember(ctx, memberID)
}

func (s *Service) ListActiveSessions(ctx context.Context) ([]*domain.Session, error) {
	return s.repo.ListActive(ctx)
}

func (s *Service) ListSessionsBySpace(ctx context.Context, spaceID string) ([]*domain.Session, error) {
	return s.repo.ListBySpace(ctx, spaceID)
}

func (s *Service) ListSessionsByMember(ctx context.Context, memberID string) ([]*domain.Session, error) {
	return s.repo.ListByMember(ctx, memberID)
}
