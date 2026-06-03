package domain_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

type fakeRuntime struct {
	kind string
}

func (f *fakeRuntime) Kind() string { return f.kind }
func (f *fakeRuntime) Start(domain.StartParams) (domain.StartSpec, error) {
	return domain.StartSpec{}, nil
}
func (f *fakeRuntime) ParseEvents([]byte) ([]domain.Event, error)              { return nil, nil }
func (f *fakeRuntime) WritePrompt(io.Writer, domain.PromptInput) error         { return nil }
func (f *fakeRuntime) WriteToolResult(io.Writer, domain.ToolResultInput) error { return nil }

func TestNewRegistry_Valid(t *testing.T) {
	r, err := domain.NewRegistry(&fakeRuntime{kind: "claude-cli"}, &fakeRuntime{kind: "codex"})
	require.NoError(t, err)

	rt, err := r.Get("claude-cli")
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, "claude-cli", rt.Kind())

	rt, err = r.Get("codex")
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, "codex", rt.Kind())
}

func TestNewRegistry_DuplicateKind(t *testing.T) {
	_, err := domain.NewRegistry(&fakeRuntime{kind: "codex"}, &fakeRuntime{kind: "codex"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate runtime kind")
}

func TestNewRegistry_ReservedKind(t *testing.T) {
	_, err := domain.NewRegistry(&fakeRuntime{kind: "internal"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

func TestNewRegistry_EmptyKind(t *testing.T) {
	_, err := domain.NewRegistry(&fakeRuntime{kind: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime kind is required")
}

func TestNewRegistry_NilRuntime(t *testing.T) {
	r, err := domain.NewRegistry(nil, &fakeRuntime{kind: "codex"})
	require.NoError(t, err)
	rt, err := r.Get("codex")
	require.NoError(t, err)
	require.NotNil(t, rt)
}

func TestRegistry_GetEmptyKind(t *testing.T) {
	r, err := domain.NewRegistry(&fakeRuntime{kind: "codex"})
	require.NoError(t, err)
	rt, err := r.Get("")
	require.NoError(t, err)
	assert.Nil(t, rt)
}

func TestRegistry_GetInternalKind(t *testing.T) {
	r, err := domain.NewRegistry(&fakeRuntime{kind: "codex"})
	require.NoError(t, err)
	rt, err := r.Get("internal")
	require.NoError(t, err)
	assert.Nil(t, rt)
}

func TestRegistry_GetUnknownKind(t *testing.T) {
	r, err := domain.NewRegistry(&fakeRuntime{kind: "codex"})
	require.NoError(t, err)
	_, err = r.Get("missing-runtime")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not supported"))
}
