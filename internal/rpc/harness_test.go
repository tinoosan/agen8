package rpc

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
	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
)

func TestRegisterHarnessDispatchesConfigOptions(t *testing.T) {
	server := newHarnessRPCServerForTest(t)
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"harness.configOptions","params":{}}`))
	require.NoError(t, err)

	var resp struct {
		Result struct {
			Harnesses []struct {
				Kind string `json:"kind"`
			} `json:"harnesses"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error)
	assert.NotEmpty(t, resp.Result.Harnesses)
}

func TestRegisterHarnessRequiresIdentity(t *testing.T) {
	server := newHarnessRPCServerForTest(t)

	raw, err := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"harness.configOptions","params":{}}`))
	require.NoError(t, err)

	var resp Response
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.NotNil(t, resp.Error)
	assert.Equal(t, CodeInvalidRequest, resp.Error.Code)
}

func TestRegisterHarnessDispatchesRunList(t *testing.T) {
	runRepo := newHarnessMemRunRepo()
	svc := newHarnessServiceWithRunRepoForTest(t, runRepo, harnessRuntime{kind: "codex"})
	reg := NewRegistry()
	require.NoError(t, RegisterHarness(reg, svc))
	server, err := NewServer(reg)
	require.NoError(t, err)
	run, err := harnessrun.Start(harnessrun.StartParams{
		ID:          "run-1",
		ProjectID:   "project-1",
		SpaceID:     "space-1",
		ChannelID:   "channel:space-1:member:member-1",
		MemberID:    "member-1",
		SessionID:   "session-1",
		HarnessKind: "codex",
		TurnID:      "turn-1",
		StartedAt:   time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NoError(t, runRepo.Save(context.Background(), run))

	raw, err := server.Handle(
		ContextWithIdentity(context.Background(), Identity{UserID: "user-1"}),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"harness.run.list","params":{"spaceId":"space-1"}}`),
	)
	require.NoError(t, err)

	var resp struct {
		Result struct {
			Runs []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"runs"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error)
	require.Len(t, resp.Result.Runs, 1)
	assert.Equal(t, "run-1", resp.Result.Runs[0].ID)
	assert.Equal(t, "running", resp.Result.Runs[0].Status)
}

func TestRegisterHarnessDispatchesTurnCancel(t *testing.T) {
	runRepo := newHarnessMemRunRepo()
	rt := newBlockingHarnessRuntime("codex")
	svc := newHarnessServiceWithRunRepoForTest(t, runRepo, rt)
	svc.SetProjectWorkdirResolver(harnessRPCWorkdirResolver{})
	reg := NewRegistry()
	require.NoError(t, RegisterHarness(reg, svc))
	server, err := NewServer(reg)
	require.NoError(t, err)
	_, err = svc.ActivateSession(context.Background(), harnessActivationParams("member-1", "space-1", "codex", "gpt-5.5", "high"))
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		_, err := svc.SendMessage(context.Background(), harnessapp.SendMessageParams{
			SpaceID:               "space-1",
			MemberID:              "member-1",
			ChannelID:             "channel:space-1:member:member-1",
			ConversationMessageID: "conversation-1",
			Text:                  "stop later",
		})
		errCh <- err
	}()

	select {
	case <-rt.started:
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not start")
	}
	raw, err := server.Handle(
		ContextWithIdentity(context.Background(), Identity{UserID: "user-1"}),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"turn.cancel","params":{"runId":"run-1","channelId":"channel:space-1:member:member-1"}}`),
	)
	require.NoError(t, err)

	var resp struct {
		Result struct {
			Run struct {
				ID              string `json:"id"`
				Status          string `json:"status"`
				StopRequestedBy string `json:"stopRequestedBy"`
			} `json:"run"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.Nil(t, resp.Error)
	assert.Equal(t, "run-1", resp.Result.Run.ID)
	assert.Equal(t, "stop_requested", resp.Result.Run.Status)
	assert.Equal(t, "user-1", resp.Result.Run.StopRequestedBy)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("runtime turn did not stop")
	}
}

func newHarnessRPCServerForTest(t *testing.T) *Server {
	t.Helper()
	svc := newHarnessServiceForTest(t)
	reg := NewRegistry()
	require.NoError(t, RegisterHarness(reg, svc))
	server, err := NewServer(reg)
	require.NoError(t, err)
	return server
}

func newHarnessServiceForTest(t *testing.T) *harnessapp.Service {
	t.Helper()
	return newHarnessServiceWithRunRepoForTest(t, newHarnessMemRunRepo(), harnessRuntime{kind: "codex"}, harnessRuntime{kind: "claude-cli"})
}

func newHarnessServiceWithRunRepoForTest(t *testing.T, runRepo harnessrun.Repository, runtimes ...domain.Runtime) *harnessapp.Service {
	t.Helper()
	seq := 0
	svc, err := harnessapp.NewService(
		domain.DefaultCatalog(),
		runtimes,
		newHarnessMemRepo(),
		runRepo,
		func() string { seq++; return fmt.Sprintf("session-%d", seq) },
		func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) },
		nil,
	)
	require.NoError(t, err)
	return svc
}

func harnessActivationParams(memberID, spaceID, kind, model, effort string) harnessapp.ActivateSessionParams {
	return harnessapp.ActivateSessionParams{
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
	}
}

type harnessRPCWorkdirResolver struct{}

func (harnessRPCWorkdirResolver) ResolveHarnessWorkdir(context.Context, string) (harnessapp.ProjectWorkdir, error) {
	return harnessapp.ProjectWorkdir{LocationID: "local", Workdir: "/tmp/project-1"}, nil
}

type harnessMemRepo struct {
	mu       sync.Mutex
	sessions map[string]*domain.Session
}

func newHarnessMemRepo() *harnessMemRepo {
	return &harnessMemRepo{sessions: make(map[string]*domain.Session)}
}

func (r *harnessMemRepo) Save(_ context.Context, session *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *session
	r.sessions[session.ID] = &cp
	return nil
}

func (r *harnessMemRepo) Get(_ context.Context, id string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := *session
	return &cp, nil
}

func (r *harnessMemRepo) GetActiveByMember(_ context.Context, memberID string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, session := range r.sessions {
		if session.MemberID == memberID && session.Status == domain.SessionActive {
			cp := *session
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *harnessMemRepo) ListActive(_ context.Context) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, session := range r.sessions {
		if session.Status == domain.SessionActive {
			cp := *session
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *harnessMemRepo) ListByMember(_ context.Context, memberID string) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, session := range r.sessions {
		if session.MemberID == memberID {
			cp := *session
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *harnessMemRepo) ListBySpace(_ context.Context, spaceID string) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Session
	for _, session := range r.sessions {
		if session.SpaceID == spaceID {
			cp := *session
			out = append(out, &cp)
		}
	}
	return out, nil
}

type harnessMemRunRepo struct {
	mu   sync.Mutex
	runs map[string]harnessrun.Run
}

func newHarnessMemRunRepo() *harnessMemRunRepo {
	return &harnessMemRunRepo{runs: map[string]harnessrun.Run{}}
}

func (r *harnessMemRunRepo) Save(_ context.Context, item harnessrun.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[item.ID] = item
	return nil
}

func (r *harnessMemRunRepo) Get(_ context.Context, id string) (*harnessrun.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.runs[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (r *harnessMemRunRepo) GetActiveBySession(_ context.Context, sessionID string) (*harnessrun.Run, error) {
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

func (r *harnessMemRunRepo) GetByTurnID(_ context.Context, turnID string) (*harnessrun.Run, error) {
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

func (r *harnessMemRunRepo) List(_ context.Context, filter harnessrun.Filter) ([]harnessrun.Run, error) {
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
		out = append(out, item)
	}
	return out, nil
}

func (r *harnessMemRunRepo) MarkRuntimeLost(context.Context) ([]harnessrun.Run, error) {
	return nil, nil
}

type harnessRuntime struct{ kind string }

func (f harnessRuntime) Kind() string { return f.kind }
func (f harnessRuntime) Start(domain.StartParams) (domain.StartSpec, error) {
	return domain.StartSpec{}, nil
}
func (f harnessRuntime) ParseEvents([]byte) ([]domain.Event, error)              { return nil, nil }
func (f harnessRuntime) WritePrompt(io.Writer, domain.PromptInput) error         { return nil }
func (f harnessRuntime) WriteToolResult(io.Writer, domain.ToolResultInput) error { return nil }

type blockingHarnessRuntime struct {
	harnessRuntime
	started chan struct{}
}

func newBlockingHarnessRuntime(kind string) *blockingHarnessRuntime {
	return &blockingHarnessRuntime{
		harnessRuntime: harnessRuntime{kind: kind},
		started:        make(chan struct{}),
	}
}

func (f *blockingHarnessRuntime) ExecuteSessionTurn(ctx context.Context, params domain.StartParams, input domain.SessionTurnInput, emit func(domain.Event)) (domain.SessionTurnResult, error) {
	close(f.started)
	if emit != nil {
		emit(domain.Event{Type: domain.EventTurnStarted, TurnID: input.TurnID, SessionRef: params.SessionRef})
	}
	<-ctx.Done()
	return domain.SessionTurnResult{}, ctx.Err()
}
