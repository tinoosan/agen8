package rpc

import (
	"context"
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

func (r *memRepo) Get(_ context.Context, id string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := *session
	return &cp, nil
}

func (r *memRepo) GetActiveByMember(_ context.Context, memberID string) (*domain.Session, error) {
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

func (r *memRepo) ListActive(_ context.Context) ([]*domain.Session, error) {
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

func (r *memRepo) ListByMember(_ context.Context, memberID string) ([]*domain.Session, error) {
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

func (r *memRepo) ListBySpace(_ context.Context, spaceID string) ([]*domain.Session, error) {
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
		out = append(out, item)
	}
	return out, nil
}

func (r *memRunRepo) MarkRuntimeLost(context.Context) ([]harnessrun.Run, error) {
	return nil, nil
}

type fakeRuntime struct{ kind string }

func (f fakeRuntime) Kind() string { return f.kind }
func (f fakeRuntime) Start(domain.StartParams) (domain.StartSpec, error) {
	return domain.StartSpec{}, nil
}
func (f fakeRuntime) ParseEvents([]byte) ([]domain.Event, error)              { return nil, nil }
func (f fakeRuntime) WritePrompt(io.Writer, domain.PromptInput) error         { return nil }
func (f fakeRuntime) WriteToolResult(io.Writer, domain.ToolResultInput) error { return nil }

type fakeWorkdirResolver struct{}

func (fakeWorkdirResolver) ResolveHarnessWorkdir(context.Context, string) (harnessapp.ProjectWorkdir, error) {
	return harnessapp.ProjectWorkdir{LocationID: "local", Workdir: "/tmp/project-1"}, nil
}

func newTestHandler(t *testing.T) (*Handler, *harnessapp.Service) {
	t.Helper()
	seq := 0
	svc, err := harnessapp.NewService(
		domain.DefaultCatalog(),
		[]domain.Runtime{fakeRuntime{kind: "codex"}, fakeRuntime{kind: "claude-cli"}},
		newMemRepo(),
		newMemRunRepo(),
		func() string { seq++; return fmt.Sprintf("session-%d", seq) },
		func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) },
		nil,
	)
	require.NoError(t, err)
	svc.SetProjectWorkdirResolver(fakeWorkdirResolver{})
	return MustNewHandler(svc), svc
}

func activationParams(memberID, spaceID, kind, model, effort string) harnessapp.ActivateSessionParams {
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

func TestConfigOptionsReturnsCatalog(t *testing.T) {
	handler, _ := newTestHandler(t)

	result, err := handler.ConfigOptions(context.Background(), ConfigOptionsParams{})
	require.NoError(t, err)

	require.NotEmpty(t, result.Harnesses)
	assert.Equal(t, "claude-cli", result.Harnesses[0].Kind)
	assert.NotEmpty(t, result.Harnesses[0].Models)
	assert.NotEmpty(t, result.Harnesses[0].Models[0].Efforts)
}

func TestSessionGetRequiresID(t *testing.T) {
	handler, _ := newTestHandler(t)

	_, err := handler.SessionGet(context.Background(), SessionGetParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sessionId is required")
}

func TestSessionListByMember(t *testing.T) {
	handler, svc := newTestHandler(t)
	ctx := context.Background()
	session, err := svc.ActivateSession(ctx, activationParams("member-1", "space-1", "codex", "gpt-5.5", "high"))
	require.NoError(t, err)

	result, err := handler.SessionList(ctx, SessionListParams{MemberID: "member-1"})
	require.NoError(t, err)

	require.Len(t, result.Sessions, 1)
	assert.Equal(t, session.ID, result.Sessions[0].ID)
}
