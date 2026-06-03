package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
	humaninputdomain "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

var testNow = time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

type memRepo struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
}

func newMemRepo() *memRepo {
	return &memRepo{sessions: make(map[string]*domain.Session)}
}

func (r *memRepo) Save(_ context.Context, session *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	r.sessions[session.ID] = &cp
	return nil
}

func (r *memRepo) Get(_ context.Context, sessionRef string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionRef]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (r *memRepo) GetActiveByMember(_ context.Context, memberID string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.sessions {
		if s.MemberID == memberID && s.Status == domain.SessionActive {
			cp := *s
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRepo) ListActive(_ context.Context) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, s := range r.sessions {
		if s.Status == domain.SessionActive {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *memRepo) ListByMember(_ context.Context, memberID string) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, s := range r.sessions {
		if s.MemberID == memberID {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *memRepo) ListBySpace(_ context.Context, spaceID string) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, s := range r.sessions {
		if s.SpaceID == spaceID {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

type memRunRepo struct {
	mu   sync.Mutex
	runs map[string]harnessrun.Run
}

func newMemRunRepo() *memRunRepo {
	return &memRunRepo{runs: map[string]harnessrun.Run{}}
}

func (r *memRunRepo) Save(_ context.Context, item harnessrun.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[item.ID] = item
	return nil
}

func (r *memRunRepo) Get(_ context.Context, id string) (*harnessrun.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.runs[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (r *memRunRepo) GetActiveBySession(_ context.Context, sessionID string) (*harnessrun.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.runs {
		if item.SessionID == sessionID && !item.IsTerminal() {
			cp := item
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRunRepo) GetByTurnID(_ context.Context, turnID string) (*harnessrun.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.runs {
		if item.TurnID == turnID || item.NativeTurnID == turnID {
			cp := item
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRunRepo) List(_ context.Context, filter harnessrun.Filter) ([]harnessrun.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []harnessrun.Run{}
	for _, item := range r.runs {
		if filter.ProjectID != "" && item.ProjectID != filter.ProjectID {
			continue
		}
		if filter.SpaceID != "" && item.SpaceID != filter.SpaceID {
			continue
		}
		if filter.ChannelID != "" && item.ChannelID != filter.ChannelID {
			continue
		}
		if filter.MemberID != "" && item.MemberID != filter.MemberID {
			continue
		}
		if filter.SessionID != "" && item.SessionID != filter.SessionID {
			continue
		}
		if len(filter.Status) > 0 {
			matched := false
			for _, status := range filter.Status {
				if item.Status == status {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *memRunRepo) MarkRuntimeLost(context.Context) ([]harnessrun.Run, error) {
	return nil, nil
}

type fakeRuntime struct{ kind string }

func (f *fakeRuntime) Kind() string { return f.kind }
func (f *fakeRuntime) Start(domain.StartParams) (domain.StartSpec, error) {
	return domain.StartSpec{}, nil
}
func (f *fakeRuntime) ParseEvents([]byte) ([]domain.Event, error)              { return nil, nil }
func (f *fakeRuntime) WritePrompt(io.Writer, domain.PromptInput) error         { return nil }
func (f *fakeRuntime) WriteToolResult(io.Writer, domain.ToolResultInput) error { return nil }

type fakeSessionRuntime struct {
	fakeRuntime
	input           domain.SessionTurnInput
	params          domain.StartParams
	events          []domain.Event
	approvalRequest *domain.ApprovalRequest
	err             error
}

type fakeSyncRuntime struct {
	fakeRuntime
	params domain.StartParams
	events []domain.Event
	done   chan struct{}
}

type spyExternalSink struct {
	mu     sync.Mutex
	events []app.ExternalSessionEvent
	done   chan struct{}
}

func (s *spyExternalSink) AppendHarnessExternalEvent(_ context.Context, event app.ExternalSessionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if s.done != nil {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
	}
	return nil
}

type invalidatingSessionRuntime struct {
	fakeSessionRuntime
	invalidated []string
}

func (f *invalidatingSessionRuntime) InvalidateSessionRef(sessionRef string) error {
	f.invalidated = append(f.invalidated, sessionRef)
	return nil
}

type blockingSessionRuntime struct {
	fakeRuntime
	started chan struct{}
}

type steeringBlockingSessionRuntime struct {
	*blockingSessionRuntime
	steered chan domain.PromptInput
}

func newBlockingSessionRuntime(kind string) *blockingSessionRuntime {
	return &blockingSessionRuntime{
		fakeRuntime: fakeRuntime{kind: kind},
		started:     make(chan struct{}),
	}
}

func newSteeringBlockingSessionRuntime(kind string) *steeringBlockingSessionRuntime {
	return &steeringBlockingSessionRuntime{
		blockingSessionRuntime: newBlockingSessionRuntime(kind),
		steered:                make(chan domain.PromptInput, 1),
	}
}

func (f *blockingSessionRuntime) ExecuteSessionTurn(ctx context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	close(f.started)
	if emit != nil {
		emit(domain.Event{Type: domain.EventTurnStarted, TurnID: input.TurnID, SessionRef: params.SessionRef})
	}
	<-ctx.Done()
	return domain.SessionTurnResult{}, ctx.Err()
}

func (f *steeringBlockingSessionRuntime) SupportsSessionSteering() bool {
	return true
}

func (f *steeringBlockingSessionRuntime) ExecuteSessionTurn(ctx context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	close(f.started)
	if emit != nil {
		emit(domain.Event{Type: domain.EventTurnStarted, TurnID: input.TurnID, SessionRef: params.SessionRef})
	}
	for {
		select {
		case steering, ok := <-input.Steering:
			if !ok {
				input.Steering = nil
				continue
			}
			f.steered <- steering
		case <-ctx.Done():
			return domain.SessionTurnResult{}, ctx.Err()
		}
	}
}

type fakeHumanInputAwaiter struct {
	pending humaninputdomain.PendingRequest
	result  json.RawMessage
}

type fakeWorkdirResolver struct {
	locationID string
	workdir    string
	err        error
}

func (f fakeWorkdirResolver) ResolveHarnessWorkdir(context.Context, string) (app.ProjectWorkdir, error) {
	if f.err != nil {
		return app.ProjectWorkdir{}, f.err
	}
	locationID := f.locationID
	if locationID == "" {
		locationID = "local"
	}
	if f.workdir != "" {
		return app.ProjectWorkdir{LocationID: locationID, Workdir: f.workdir}, nil
	}
	return app.ProjectWorkdir{LocationID: locationID, Workdir: "/tmp/project-1"}, nil
}

type fakeRuntimeHostResolver struct {
	locationID  string
	harnessKind string
	endpointURL string
	mcpBaseURL  string
	diagnostics string
	err         error
}

func (f *fakeRuntimeHostResolver) ResolveRuntimeHost(ctx context.Context, input app.RuntimeHostRequest) (app.RuntimeHost, error) {
	f.locationID = input.LocationID
	f.harnessKind = input.HarnessKind
	if f.err != nil {
		return app.RuntimeHost{}, f.err
	}
	endpoint := f.endpointURL
	if endpoint == "" {
		endpoint = "ws://127.0.0.1:49152/codex"
	}
	return app.RuntimeHost{AppServerURL: endpoint, MCPBaseURL: f.mcpBaseURL, Diagnostics: f.diagnostics}, nil
}

type fakeAttachmentStager struct {
	request app.AttachmentStageRequest
	uri     string
	err     error
}

func (f *fakeAttachmentStager) StageAttachments(_ context.Context, request app.AttachmentStageRequest) ([]app.PromptAttachment, error) {
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	out := append([]app.PromptAttachment(nil), request.Attachments...)
	for i := range out {
		if f.uri != "" {
			out[i].URI = f.uri
		}
	}
	return out, nil
}

type fakeMCPConfigFormatter struct{}

func (fakeMCPConfigFormatter) FormatMCPServers(_ context.Context, request app.MCPConfigRequest) ([]string, error) {
	if request.RawURL != "" {
		return []string{request.HarnessKind + ":" + request.RawURL}, nil
	}
	return []string{request.HarnessKind + ":" + request.BaseURL + "/mcp?token=" + request.Token}, nil
}

func (f *fakeHumanInputAwaiter) Await(_ context.Context, pending humaninputdomain.PendingRequest) (json.RawMessage, error) {
	f.pending = pending
	if len(f.result) > 0 {
		return f.result, nil
	}
	return json.RawMessage(`{"decision":"approve"}`), nil
}

func (f *fakeSessionRuntime) ExecuteSessionTurn(_ context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	f.params = params
	f.input = input
	if f.approvalRequest != nil {
		if params.ApprovalHandler == nil {
			return domain.SessionTurnResult{}, fmt.Errorf("approval handler is required")
		}
		if _, err := params.ApprovalHandler(context.Background(), *f.approvalRequest); err != nil {
			return domain.SessionTurnResult{}, err
		}
	}
	if emit != nil {
		events := f.events
		if len(events) == 0 {
			events = []domain.Event{
				{Type: domain.EventTurnStarted, TurnID: "turn-1", SessionRef: "thread-1"},
				{Type: domain.EventText, TurnID: "turn-1", Text: "thinking", Data: map[string]string{"kind": "reasoning"}},
				{Type: domain.EventText, TurnID: "turn-1", Text: "done", Data: map[string]string{"kind": "assistant"}},
				{Type: domain.EventTurnCompleted, TurnID: "turn-1", SessionRef: "thread-1"},
			}
		}
		for _, ev := range events {
			emit(ev)
		}
	}
	if f.err != nil {
		return domain.SessionTurnResult{}, f.err
	}
	return domain.SessionTurnResult{}, nil
}

func (f *fakeSyncRuntime) SyncSession(_ context.Context, params domain.StartParams, emit func(domain.Event)) error {
	f.params = params
	for _, ev := range f.events {
		if emit != nil {
			emit(ev)
		}
	}
	if f.done != nil {
		close(f.done)
	}
	return nil
}

func newTestService(t *testing.T) (*app.Service, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	seq := 0
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{&fakeRuntime{kind: "claude-cli"}, &fakeRuntime{kind: "codex"}},
		repo,
		newMemRunRepo(),
		func() string { seq++; return fmt.Sprintf("sess-%d", seq) },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	return svc, repo
}

func activationParams(memberID, spaceID, kind, model, effort string) app.ActivateSessionParams {
	return app.ActivateSessionParams{
		ProjectID:      "project-1",
		MemberID:       memberID,
		SpaceID:        spaceID,
		ChannelID:      "channel:" + spaceID + ":member:" + memberID,
		DisplayName:    "Test Member",
		MemberType:     "worker",
		LifecycleState: "active",
		HarnessKind:    kind,
		Model:          model,
		Effort:         effort,
		MCPToken:       "mcp-token-" + memberID,
		MCPServers:     []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=mcp-token-` + memberID + `"`},
	}
}

func TestService_SendMessageRoutesToActiveMemberSession(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{fakeRuntime: fakeRuntime{kind: "codex"}}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
	})
	require.NoError(t, err)
	assert.Equal(t, "sess-1", result.SessionID)
	assert.Equal(t, "turn-1", result.TurnID)
	assert.Equal(t, "delivered", result.Delivery)
	assert.Equal(t, "done", result.Text)
	assert.Equal(t, "please help", rt.input.Text)
	assert.Equal(t, "gpt-5.5", rt.params.Model)
	assert.Equal(t, "medium", rt.params.ReasoningEffort)
	assert.Equal(t, "/tmp/project-1", rt.params.Workdir)
	assert.Contains(t, rt.params.SystemPrompt, `member id="member-1"`)
	assert.Equal(t, []string{`mcp_servers.agen8.url="http://127.0.0.1:7777/mcp?token=mcp-token-member-1"`}, rt.params.MCPServers)

	session, err := svc.GetSession(context.Background(), "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "thread-1", session.Ref)
}

func TestService_RequestStopCancelsActiveRun(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := newBlockingSessionRuntime("codex")
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	resultCh := make(chan app.SendMessageResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
			SpaceID:               "space-1",
			MemberID:              "member-1",
			ChannelID:             "channel:space-1:member:member-1",
			ConversationMessageID: "conversation-1",
			Text:                  "please help",
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case <-rt.started:
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not start")
	}

	stop, err := svc.RequestStop(context.Background(), app.RequestStopParams{
		RunID:       "run-1",
		ChannelID:   "channel:space-1:member:member-1",
		RequestedBy: "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, harnessrun.StatusStopRequested, stop.Status)
	assert.Equal(t, "user-1", stop.StopRequestedBy)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case result := <-resultCh:
		assert.Equal(t, "run-1", result.RunID)
		assert.Equal(t, "delivered", result.Delivery)
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not stop")
	}

	stored, err := runRepo.Get(context.Background(), "run-1")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, harnessrun.StatusCanceled, stored.Status)
	require.NotNil(t, stored.StopRequestedAt)
	require.NotNil(t, stored.CompletedAt)
}

func TestService_SendMessageRejectsActiveRunUnlessSteeringIsAllowed(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := newSteeringBlockingSessionRuntime("codex")
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.SendMessage(context.Background(), app.SendMessageParams{
			SpaceID:               "space-1",
			MemberID:              "member-1",
			ChannelID:             "channel:space-1:member:member-1",
			ConversationMessageID: "conversation-1",
			Text:                  "please help",
		})
		errCh <- err
	}()

	select {
	case <-rt.started:
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not start")
	}

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-2",
		Text:                  "new instruction",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `harness session "sess-1" already has active run "run-1"`)

	_, err = svc.RequestStop(context.Background(), app.RequestStopParams{
		RunID:       "run-1",
		ChannelID:   "channel:space-1:member:member-1",
		RequestedBy: "user-1",
	})
	require.NoError(t, err)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not stop")
	}
}

func TestService_SendMessageSteersActiveRunWhenExplicitlyAllowed(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := newSteeringBlockingSessionRuntime("codex")
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.SendMessage(context.Background(), app.SendMessageParams{
			SpaceID:               "space-1",
			MemberID:              "member-1",
			ChannelID:             "channel:space-1:member:member-1",
			ConversationMessageID: "conversation-1",
			Text:                  "please help",
		})
		errCh <- err
	}()

	select {
	case <-rt.started:
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not start")
	}

	result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-2",
		Text:                  "new instruction",
		AllowSteering:         true,
	})
	require.NoError(t, err)
	assert.Equal(t, "sess-1", result.SessionID)
	assert.Equal(t, "run-1", result.RunID)
	assert.Equal(t, "turn-conversation-1", result.TurnID)
	assert.Equal(t, "steered", result.Delivery)

	select {
	case steering := <-rt.steered:
		assert.Equal(t, "turn-conversation-1", steering.TurnID)
		assert.Equal(t, "new instruction", steering.Text)
	case <-time.After(time.Second):
		t.Fatal("runtime did not receive steering input")
	}

	_, err = svc.RequestStop(context.Background(), app.RequestStopParams{
		RunID:       "run-1",
		ChannelID:   "channel:space-1:member:member-1",
		RequestedBy: "user-1",
	})
	require.NoError(t, err)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not stop")
	}
}

func TestService_SendMessageRejectsActiveRunWhenRuntimeDoesNotSupportSteering(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := newBlockingSessionRuntime("claude-cli")
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.SendMessage(context.Background(), app.SendMessageParams{
			SpaceID:               "space-1",
			MemberID:              "member-1",
			ChannelID:             "channel:space-1:member:member-1",
			ConversationMessageID: "conversation-1",
			Text:                  "please help",
		})
		errCh <- err
	}()

	select {
	case <-rt.started:
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not start")
	}

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-2",
		Text:                  "new instruction",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `harness session "sess-1" already has active run "run-1"`)

	_, err = svc.RequestStop(context.Background(), app.RequestStopParams{
		RunID:       "run-1",
		ChannelID:   "channel:space-1:member:member-1",
		RequestedBy: "user-1",
	})
	require.NoError(t, err)
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not stop")
	}
}

func TestService_SendMessageRecoversStaleActiveRunWithoutCancelHandle(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := &fakeSessionRuntime{fakeRuntime: fakeRuntime{kind: "codex"}}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	session, err := svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	staleRun, err := harnessrun.Start(harnessrun.StartParams{
		ID:               "run-stale",
		ProjectID:        session.ProjectID,
		SpaceID:          session.SpaceID,
		ChannelID:        session.ChannelID,
		MemberID:         session.MemberID,
		SessionID:        session.ID,
		HarnessKind:      session.Kind,
		NativeSessionRef: "thread-stale",
		TurnID:           "turn-conversation-stale",
		StartedAt:        testNow.Add(-time.Minute),
	})
	require.NoError(t, err)
	require.NoError(t, runRepo.Save(context.Background(), staleRun))

	result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-fresh",
		Text:                  "please continue",
	})
	require.NoError(t, err)
	assert.Equal(t, "run-fresh", result.RunID)
	assert.Equal(t, "delivered", result.Delivery)

	storedStale, err := runRepo.Get(context.Background(), "run-stale")
	require.NoError(t, err)
	require.NotNil(t, storedStale)
	assert.Equal(t, harnessrun.StatusFailed, storedStale.Status)
	assert.Equal(t, "runtime_lost", storedStale.Error)
	require.NotNil(t, storedStale.CompletedAt)

	storedFresh, err := runRepo.Get(context.Background(), "run-fresh")
	require.NoError(t, err)
	require.NotNil(t, storedFresh)
	assert.Equal(t, harnessrun.StatusCompleted, storedFresh.Status)
	require.NotNil(t, storedFresh.CompletedAt)
}

func TestService_SendMessageRoutesApprovalThroughHumanInput(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		approvalRequest: &domain.ApprovalRequest{
			ApprovalID: "approval-1",
			ToolCallID: "tool-1",
			ToolName:   "bash",
			Command:    "rm -rf dist",
			Summary:    "Approve command execution",
			Method:     "item/commandExecution/requestApproval",
			Data:       map[string]string{"cwd": "/tmp/project-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	awaiter := &fakeHumanInputAwaiter{result: json.RawMessage(`{"decision":"approve","note":"looks good"}`)}
	svc.SetHumanInputAwaiter(awaiter)
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please run the command",
	})
	require.NoError(t, err)

	assert.Equal(t, "tool-1", awaiter.pending.ToolCallID)
	assert.Equal(t, "bash", awaiter.pending.ToolName)
	assert.Equal(t, "project-1", awaiter.pending.ProjectID)
	assert.Equal(t, "space-1", awaiter.pending.SpaceID)
	assert.Equal(t, "member-1", awaiter.pending.MemberID)
	assert.Equal(t, "channel:space-1:member:member-1", awaiter.pending.ChannelID)
	assert.Equal(t, humaninputdomain.PrimitiveApproveReject, awaiter.pending.Declaration.Kind)

	var payload humaninputdomain.ApproveRejectPayload
	require.NoError(t, json.Unmarshal(awaiter.pending.Declaration.Payload, &payload))
	assert.Equal(t, "Approve command execution", payload.Title)
	assert.Equal(t, "rm -rf dist", payload.Description)
	assert.Contains(t, payload.Context, "method=item/commandExecution/requestApproval")
	assert.Contains(t, payload.Context, "approvalId=approval-1")
	assert.Contains(t, payload.Context, "cwd=/tmp/project-1")
	assert.Contains(t, payload.Context, "harness=codex")
	assert.Contains(t, payload.Context, "memberId=member-1")
	assert.Contains(t, payload.Context, "spaceId=space-1")
	assert.Contains(t, payload.Context, "channelId=channel:space-1:member:member-1")
	assert.Contains(t, payload.Context, "locationId=local")
	assert.Contains(t, payload.Context, "workdir=/tmp/project-1")
}

func TestService_SendMessageUsesRemoteRuntimeHostForRemoteCodexSession(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{fakeRuntime: fakeRuntime{kind: "codex"}}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{locationID: "loc-ssh", workdir: "/srv/playground"})
	hosts := &fakeRuntimeHostResolver{
		endpointURL: "ws://127.0.0.1:49152/codex",
		mcpBaseURL:  "http://127.0.0.1:38123",
		diagnostics: `{"bridgeVersion":"bridge-v4","codexPath":"/home/dev/.local/bin/codex","codexVersion":"codex-cli 1.2.3"}`,
	}
	svc.SetRuntimeHostResolver(hosts)
	svc.SetMCPConfigFormatter(fakeMCPConfigFormatter{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
	})
	require.NoError(t, err)
	assert.Equal(t, "loc-ssh", hosts.locationID)
	assert.Equal(t, "codex", hosts.harnessKind)
	assert.Equal(t, "ws://127.0.0.1:49152/codex", rt.params.AppServerURL)
	assert.Equal(t, `{"bridgeVersion":"bridge-v4","codexPath":"/home/dev/.local/bin/codex","codexVersion":"codex-cli 1.2.3"}`, rt.params.RuntimeHostDiagnostics)
	assert.Equal(t, []string{"codex:http://127.0.0.1:38123/mcp?token=mcp-token-member-1"}, rt.params.MCPServers)
	assert.Nil(t, rt.params.CommandRunner)
	assert.Equal(t, "/srv/playground", rt.params.Workdir)
}

func TestService_SendMessageStagesAttachmentsForRemoteSession(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{fakeRuntime: fakeRuntime{kind: "codex"}}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{locationID: "loc-ssh", workdir: "/srv/playground"})
	svc.SetRuntimeHostResolver(&fakeRuntimeHostResolver{})
	stager := &fakeAttachmentStager{uri: "/srv/playground/.agen8/conversation-attachments/runtime/attachment-1-screen.png"}
	svc.SetAttachmentStager(stager)
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "inspect",
		Attachments: []app.PromptAttachment{{
			ID:        "attachment-1",
			Name:      "screen.png",
			MediaType: "image/png",
			SizeBytes: 12,
			URI:       "/Users/me/.agen8/conversation-attachments/screen.png",
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, "loc-ssh", stager.request.LocationID)
	assert.Equal(t, "/srv/playground", stager.request.Workdir)
	require.Len(t, rt.input.Attachments, 1)
	assert.Equal(t, "/srv/playground/.agen8/conversation-attachments/runtime/attachment-1-screen.png", rt.input.Attachments[0].URI)
}

func TestService_SendMessageDoesNotRequireCommandRunnerForLocalSession(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{fakeRuntime: fakeRuntime{kind: "claude-cli"}}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{locationID: "local", workdir: "/tmp/project-1"})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
	})
	require.NoError(t, err)
	assert.Nil(t, rt.params.CommandRunner)
}

func TestService_SendMessageResetsClaudeSessionRefAfterThinkingBlockError(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	rt := &fakeSessionRuntime{
		fakeRuntime: fakeRuntime{kind: "claude-cli"},
		events: []domain.Event{
			{Type: domain.EventTurnStarted, TurnID: "turn-1", SessionRef: "thread-1"},
		},
		err: fmt.Errorf("claude-cli session turn failed: API Error: 400 messages.1.content.3: `thinking` or `redacted_thinking` blocks in the latest assistant message cannot be modified"),
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		runRepo,
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	session, err := svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-8", "medium"))
	require.NoError(t, err)
	session.Ref = "thread-1"
	require.NoError(t, repo.Save(context.Background(), session))

	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "continue",
	})
	require.Error(t, err)

	got, err := svc.GetSession(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Ref)
	stored, err := runRepo.Get(context.Background(), "run-1")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, harnessrun.StatusFailed, stored.Status)
}

func TestService_SendMessageAppendsAssistantDeltasVerbatim(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		events: []domain.Event{
			{Type: domain.EventTurnStarted, TurnID: "turn-1", SessionRef: "thread-1"},
			{Type: domain.EventText, TurnID: "turn-1", Text: "Cluster: Kubernetes `v1.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "10.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "90` at `192.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "168.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "1.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "110`.", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "turn-1", SessionRef: "thread-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	var streamed []string
	result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
		OnAssistantDelta: func(_ context.Context, delta app.AssistantDelta) error {
			streamed = append(streamed, delta.Text)
			return nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Cluster: Kubernetes `v1.10.90` at `192.168.1.110`.", result.Text)
	assert.Equal(t, []string{
		"Cluster: Kubernetes `v1.",
		"10.",
		"90` at `192.",
		"168.",
		"1.",
		"110`.",
	}, streamed)
}

func TestService_SendMessageEmitsToolActivity(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		events: []domain.Event{
			{Type: domain.EventTurnStarted, TurnID: "turn-1", SessionRef: "thread-1"},
			{Type: domain.EventToolCall, TurnID: "turn-1", ToolCallID: "call-1", ToolName: "agen8/space", Data: map[string]string{"status": "in_progress", "input": `{"action":"members"}`}},
			{Type: domain.EventToolResult, TurnID: "turn-1", ToolCallID: "call-1", ToolName: "agen8/space", Text: "OK", Data: map[string]string{"status": "completed", "result": "OK"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "done", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "turn-1", SessionRef: "thread-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	var activities []app.ActivityEvent
	_, err = svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
		OnActivity: func(_ context.Context, activity app.ActivityEvent) error {
			activities = append(activities, activity)
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, activities, 4)
	assert.Equal(t, "sess-1", activities[0].SessionID)
	assert.Equal(t, "turn-conversation-1", activities[0].TurnID)
	assert.Equal(t, "harness.run.started", activities[0].ToolName)
	assert.Equal(t, 1, activities[0].Sequence)
	assert.Equal(t, "running", activities[0].Status)
	assert.Equal(t, "turn-1", activities[1].TurnID)
	assert.Equal(t, "call-1", activities[1].ToolCallID)
	assert.Equal(t, "agen8/space", activities[1].ToolName)
	assert.Equal(t, 2, activities[1].Sequence)
	assert.Equal(t, "in_progress", activities[1].Status)
	assert.Equal(t, 3, activities[2].Sequence)
	assert.Equal(t, "completed", activities[2].Status)
	assert.Equal(t, "OK", activities[2].Text)
	assert.Equal(t, "harness.run.completed", activities[3].ToolName)
	assert.Equal(t, "completed", activities[3].Status)
}

func TestService_SendMessageEmitsThinkingDelta(t *testing.T) {
	repo := newMemRepo()
	rt := &fakeSessionRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		events: []domain.Event{
			{Type: domain.EventTurnStarted, TurnID: "turn-1", SessionRef: "thread-1"},
			{Type: domain.EventText, TurnID: "turn-1", Text: "Checking the plan", Data: map[string]string{"kind": "reasoning", "itemId": "reason-1"}},
			{Type: domain.EventText, TurnID: "turn-1", Text: "done", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "turn-1", SessionRef: "thread-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	_, err = svc.ActivateSession(context.Background(), activationParams("member-1", "space-1", "codex", "gpt-5.5", "medium"))
	require.NoError(t, err)

	var thinking []app.ThinkingDelta
	var assistant []app.AssistantDelta
	result, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
		OnThinkingDelta: func(_ context.Context, delta app.ThinkingDelta) error {
			thinking = append(thinking, delta)
			return nil
		},
		OnAssistantDelta: func(_ context.Context, delta app.AssistantDelta) error {
			assistant = append(assistant, delta)
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, thinking, 1)
	assert.Equal(t, "sess-1", thinking[0].SessionID)
	assert.Equal(t, "turn-1", thinking[0].TurnID)
	assert.Equal(t, "Checking the plan", thinking[0].Text)
	assert.Equal(t, "reason-1", thinking[0].Data["itemId"])
	require.Len(t, assistant, 1)
	assert.Equal(t, "done", assistant[0].Text)
	assert.Equal(t, "done", result.Text)
}

func TestService_SendMessageRequiresActiveSession(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.SendMessage(context.Background(), app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-missing",
		ChannelID:             "channel:space-1:member:member-missing",
		ConversationMessageID: "conversation-1",
		Text:                  "please help",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active harness session")
}

func TestNewService_RequiredDeps(t *testing.T) {
	repo := newMemRepo()
	idGen := func() string { return "x" }
	clock := func() time.Time { return testNow }
	runtimes := []domain.Runtime{&fakeRuntime{kind: "codex"}}

	_, err := app.NewService(nil, runtimes, repo, newMemRunRepo(), idGen, clock, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog is required")

	_, err = app.NewService(domain.DefaultCatalog(), runtimes, nil, newMemRunRepo(), idGen, clock, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session repository is required")

	_, err = app.NewService(domain.DefaultCatalog(), runtimes, repo, nil, idGen, clock, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run repository is required")

	_, err = app.NewService(domain.DefaultCatalog(), runtimes, repo, newMemRunRepo(), nil, clock, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id generator is required")

	_, err = app.NewService(domain.DefaultCatalog(), runtimes, repo, newMemRunRepo(), idGen, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clock is required")
}

func TestService_ValidateConfig(t *testing.T) {
	svc, _ := newTestService(t)
	require.NoError(t, svc.ValidateConfig("claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, svc.ValidateConfig("claude-cli", "claude-opus-4-8", "high"))
	require.Error(t, svc.ValidateConfig("unknown", "model", "high"))
}

func TestService_SupportedQueries(t *testing.T) {
	svc, _ := newTestService(t)
	assert.Contains(t, svc.SupportedHarnesses(), "claude-cli")
	assert.Contains(t, svc.SupportedHarnesses(), "codex")
	assert.Contains(t, svc.SupportedModels("claude-cli"), "claude-opus-4-8")
	assert.Contains(t, svc.SupportedModels("claude-cli"), "claude-opus-4-7")
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, svc.SupportedEfforts("claude-cli", "claude-opus-4-8"))
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, svc.SupportedEfforts("codex", "gpt-5.5"))
}

func TestService_GetRuntime(t *testing.T) {
	svc, _ := newTestService(t)
	rt, err := svc.GetRuntime("claude-cli")
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, "claude-cli", rt.Kind())
}

func TestService_ActivateSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)
	assert.Equal(t, "sess-1", session.ID)
	assert.Equal(t, domain.SessionActive, session.Status)
	assert.Equal(t, "claude-cli", session.Kind)
	assert.Equal(t, "claude-opus-4-7", session.Model)
	assert.Equal(t, "high", session.Effort)
	assert.Equal(t, "channel:space-1:member:member-1", session.ChannelID)
	assert.Contains(t, session.SystemPrompt, `display_name="Test Member"`)
	assert.Equal(t, "mcp-token-member-1", session.MCPToken)
}

func TestService_ActivateSession_RejectsDuplicate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	_, err = svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has an active session")
}

func TestService_DeactivateSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	err = svc.DeactivateSession(ctx, session.ID, domain.ReasonShutdown, "")
	require.NoError(t, err)

	got, err := svc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionInactive, got.Status)
	assert.Equal(t, domain.ReasonShutdown, got.InactiveReason)
}

func TestService_DeactivateSession_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	err := svc.DeactivateSession(context.Background(), "nonexistent", domain.ReasonShutdown, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestService_ReactivateSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	err = svc.DeactivateSession(ctx, session.ID, domain.ReasonCrashed, "segfault")
	require.NoError(t, err)

	reactivated, err := svc.ReactivateSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionActive, reactivated.Status)
	assert.Empty(t, reactivated.InactiveReason)
}

func TestService_ReactivateSession_BlockedByOtherActive(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	s1, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)
	err = svc.DeactivateSession(ctx, s1.ID, domain.ReasonShutdown, "")
	require.NoError(t, err)

	_, err = svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	_, err = svc.ReactivateSession(ctx, s1.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has an active session")
}

func TestService_UpdateSessionConfig(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	updated, err := svc.UpdateSessionConfig(ctx, session.ID, "claude-sonnet-4-6", "max")
	require.NoError(t, err)

	assert.Equal(t, "claude-cli", updated.Kind)
	assert.Equal(t, "claude-sonnet-4-6", updated.Model)
	assert.Equal(t, "max", updated.Effort)

	got, err := svc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-6", got.Model)
	assert.Equal(t, "max", got.Effort)
}

func TestService_UpdateSessionRuntimeContextInvalidatesLiveRuntimeSession(t *testing.T) {
	repo := newMemRepo()
	rt := &invalidatingSessionRuntime{
		fakeSessionRuntime: fakeSessionRuntime{
			fakeRuntime: fakeRuntime{kind: "codex"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{rt},
		repo,
		newMemRunRepo(),
		func() string { return "sess-1" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "codex", "gpt-5.4", "medium"))
	require.NoError(t, err)
	_, err = svc.SendMessage(ctx, app.SendMessageParams{
		SpaceID:               "space-1",
		MemberID:              "member-1",
		ChannelID:             "channel:space-1:member:member-1",
		ConversationMessageID: "msg-1",
		SenderType:            "user",
		SenderID:              "user-1",
		Text:                  "hello",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateSessionRuntimeContext(ctx, session.ID, activationParams("member-1", "space-1", "codex", "gpt-5.5", "low"))
	require.NoError(t, err)

	assert.Equal(t, "gpt-5.5", updated.Model)
	assert.Equal(t, "low", updated.Effort)
	assert.Equal(t, []string{"thread-1"}, rt.invalidated)
}

func TestService_UpdateSessionConfig_ValidatesAgainstSessionKind(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	_, err = svc.UpdateSessionConfig(ctx, session.ID, "gpt-5.5", "high")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestService_RecordUsage(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	err = svc.RecordUsage(ctx, session.ID, 100, 50)
	require.NoError(t, err)
	err = svc.RecordUsage(ctx, session.ID, 200, 75)
	require.NoError(t, err)

	got, err := svc.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(300), got.TokensIn)
	assert.Equal(t, int64(125), got.TokensOut)
}

func TestService_RecordUsage_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	err := svc.RecordUsage(context.Background(), "nonexistent", 100, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestService_GetActiveSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	got, err := svc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	assert.Nil(t, got)

	_, err = svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	got, err = svc.GetActiveSession(ctx, "member-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "member-1", got.MemberID)
}

func TestService_ListSessionsBySpace(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)
	_, err = svc.ActivateSession(ctx, activationParams("member-2", "space-1", "codex", "gpt-5.5", "high"))
	require.NoError(t, err)
	_, err = svc.ActivateSession(ctx, activationParams("member-3", "space-2", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)

	sessions, err := svc.ListSessionsBySpace(ctx, "space-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestService_ListSessionsByMember(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, err)
	_, err = svc.ActivateSession(ctx, activationParams("member-2", "space-1", "codex", "gpt-5.5", "high"))
	require.NoError(t, err)

	sessions, err := svc.ListSessionsByMember(ctx, "member-1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "member-1", sessions[0].MemberID)
}

func TestService_StartExternalSessionSyncForActiveSessions(t *testing.T) {
	repo := newMemRepo()
	done := make(chan struct{})
	runtime := &fakeSyncRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		done:        done,
		events: []domain.Event{
			{Type: domain.EventText, TurnID: "native-turn-1", SessionRef: "thread-1", Text: "external prompt", Data: map[string]string{"kind": "user"}},
			{Type: domain.EventText, TurnID: "native-turn-1", SessionRef: "thread-1", Text: "external response", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "native-turn-1", SessionRef: "thread-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{runtime},
		repo,
		newMemRunRepo(),
		func() string { return "session-new" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	sink := &spyExternalSink{done: make(chan struct{})}
	svc.SetExternalSessionEventSink(sink)
	require.NoError(t, repo.Save(context.Background(), &domain.Session{
		ID:             "session-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		ChannelID:      "channel:space-1:member:member-1",
		MemberID:       "member-1",
		Kind:           "codex",
		Model:          "gpt-5.5",
		Effort:         "medium",
		PermissionMode: "codex/full-access",
		Ref:            "thread-1",
		Status:         domain.SessionActive,
	}))

	require.NoError(t, svc.StartExternalSessionSyncForActiveSessions(context.Background()))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sync runtime was not called")
	}
	select {
	case <-sink.done:
	case <-time.After(time.Second):
		t.Fatal("external sink was not called")
	}

	assert.Equal(t, "thread-1", runtime.params.SessionRef)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	require.NotEmpty(t, sink.events)
	assert.Equal(t, "external prompt", sink.events[0].UserText)
	assert.Equal(t, "native-turn-1", sink.events[0].TurnID)
	require.Len(t, sink.events, 3)
	assert.Equal(t, "external response", sink.events[1].Text)
	assert.Equal(t, "native-turn-1", sink.events[1].TurnID)
}

func TestService_StartExternalSessionSyncForActiveSessionsSkipsManagedRunTurns(t *testing.T) {
	repo := newMemRepo()
	runRepo := newMemRunRepo()
	done := make(chan struct{})
	runtime := &fakeSyncRuntime{
		fakeRuntime: fakeRuntime{kind: "codex"},
		done:        done,
		events: []domain.Event{
			{Type: domain.EventText, TurnID: "managed-native-turn", SessionRef: "thread-1", Text: "managed response", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "managed-native-turn", SessionRef: "thread-1"},
			{Type: domain.EventText, TurnID: "native-turn-1", SessionRef: "thread-1", Text: "external prompt", Data: map[string]string{"kind": "user"}},
			{Type: domain.EventText, TurnID: "native-turn-1", SessionRef: "thread-1", Text: "external response", Data: map[string]string{"kind": "assistant"}},
			{Type: domain.EventTurnCompleted, TurnID: "native-turn-1", SessionRef: "thread-1"},
		},
	}
	svc, err := app.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{runtime},
		repo,
		runRepo,
		func() string { return "session-new" },
		func() time.Time { return testNow },
		nil,
	)
	require.NoError(t, err)
	sink := &spyExternalSink{done: make(chan struct{})}
	svc.SetExternalSessionEventSink(sink)
	require.NoError(t, repo.Save(context.Background(), &domain.Session{
		ID:             "session-1",
		ProjectID:      "project-1",
		SpaceID:        "space-1",
		ChannelID:      "channel:space-1:member:member-1",
		MemberID:       "member-1",
		Kind:           "codex",
		Model:          "gpt-5.5",
		Effort:         "medium",
		PermissionMode: "codex/full-access",
		Ref:            "thread-1",
		Status:         domain.SessionActive,
	}))
	managedRun, err := harnessrun.Start(harnessrun.StartParams{
		ID:               "run-1",
		ProjectID:        "project-1",
		SpaceID:          "space-1",
		ChannelID:        "channel:space-1:member:member-1",
		MemberID:         "member-1",
		SessionID:        "session-1",
		HarnessKind:      "codex",
		NativeSessionRef: "thread-1",
		TurnID:           "turn-conversation-1",
		StartedAt:        testNow,
	})
	require.NoError(t, err)
	require.NoError(t, managedRun.SetNativeTurnID("managed-native-turn"))
	require.NoError(t, runRepo.Save(context.Background(), managedRun))

	require.NoError(t, svc.StartExternalSessionSyncForActiveSessions(context.Background()))
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sync runtime was not called")
	}
	select {
	case <-sink.done:
	case <-time.After(time.Second):
		t.Fatal("external sink was not called")
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	require.Len(t, sink.events, 3)
	assert.Equal(t, "native-turn-1", sink.events[0].TurnID)
	assert.Equal(t, "external prompt", sink.events[0].UserText)
	assert.Equal(t, "native-turn-1", sink.events[1].TurnID)
	assert.Equal(t, "external response", sink.events[1].Text)
	assert.Equal(t, "native-turn-1", sink.events[2].TurnID)
	assert.True(t, sink.events[2].Completed)
}
