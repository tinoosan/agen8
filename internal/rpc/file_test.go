package rpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8/internal/core/types"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
	fileinfra "github.com/tinoosan/agen8/internal/services/file/infra"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
)

func TestRegisterFileDispatchesListDirAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "notes.md"), []byte("# Notes\n"), 0o644))
	server := newFileRPCServerForTest(t, projectRoot)

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"files.listDir",
		"params":{"projectId":"project-test","path":"/project"}
	}`))
	require.NoError(t, err)
	var listResp struct {
		Result struct {
			Entries []struct {
				Path string `json:"path"`
			} `json:"entries"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &listResp))
	require.Nil(t, listResp.Error)
	require.Equal(t, "/project/notes.md", listResp.Result.Entries[0].Path)

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc":"2.0",
		"id":2,
		"method":"files.get",
		"params":{"projectId":"project-test","path":"/project/notes.md"}
	}`))
	require.NoError(t, err)
	var getResp struct {
		Result struct {
			Content     string `json:"content"`
			ContentKind string `json:"contentKind"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &getResp))
	require.Nil(t, getResp.Error)
	require.Equal(t, "# Notes\n", getResp.Result.Content)
	require.Equal(t, "text", getResp.Result.ContentKind)
}

func TestRegisterFileGetRequiresPath(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	server := newFileRPCServerForTest(t, projectRoot)

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"files.get",
		"params":{"projectId":"project-test"}
	}`))
	require.NoError(t, err)
	var resp Response
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, CodeInvalidParams, resp.Error.Code)
}

func newFileRPCServerForTest(t *testing.T, projectRoot string) *Server {
	t.Helper()
	svc, err := fileapp.NewService(fileapp.Config{
		Files:    fileinfra.NewLocalRepository(),
		Projects: rpcFileProjectLoader{root: projectRoot},
	})
	require.NoError(t, err)
	reg := NewRegistry()
	require.NoError(t, RegisterFile(reg, svc))
	server, err := NewServer(reg)
	require.NoError(t, err)
	return server
}

type rpcFileProjectLoader struct {
	root string
}

func (l rpcFileProjectLoader) GetProject(_ context.Context, projectID types.ProjectID) (projectdomain.Project, error) {
	projects, err := l.ListProjects(context.Background(), projectdomain.Filter{})
	if err != nil {
		return projectdomain.Project{}, err
	}
	for _, project := range projects {
		if project.ID() == projectID {
			return project, nil
		}
	}
	return projectdomain.Project{}, os.ErrNotExist
}

func (l rpcFileProjectLoader) ListProjects(context.Context, projectdomain.Filter) ([]projectdomain.Project, error) {
	project, err := projectdomain.New(projectdomain.NewInput{
		ID:        types.ProjectID("project-test"),
		Root:      l.root,
		CreatedAt: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		return nil, err
	}
	return []projectdomain.Project{project}, nil
}
