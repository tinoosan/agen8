package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	taskapp "github.com/tinoosan/agen8-mcp-server/internal/services/task/app"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	taskinfra "github.com/tinoosan/agen8-mcp-server/internal/services/task/infra"
	taskrpc "github.com/tinoosan/agen8-mcp-server/internal/services/task/rpc"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRegisterTaskListResolvesLegacyLabelsAfterMCPRehome(t *testing.T) {
	projectSvc, taskRepo, taskSvc := newRPCTaskStack(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repo")

	legacy, err := projectSvc.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "local",
		ProjectRoot: root,
		DisplayName: "codex",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("legacy register: %v", err)
	}
	registered, err := projectSvc.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Codex backend engineer",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("register real user: %v", err)
	}
	if registered.MemberID != legacy.MemberID {
		t.Fatalf("member id changed from %q to %q", legacy.MemberID, registered.MemberID)
	}

	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	err = taskRepo.CreateTask(ctx, taskdomain.Task{
		ID:          taskdomain.TaskID("task-legacy"),
		ProjectID:   types.ProjectID(registered.ProjectID),
		AssignedTo:  member.ID(registered.MemberID),
		CreatedBy:   registered.MemberID,
		Title:       "Legacy task",
		Description: "Stored before task labels were stamped",
		Status:      taskdomain.TaskStatusSucceeded,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	})
	if err != nil {
		t.Fatalf("create legacy task: %v", err)
	}

	reg := NewRegistry()
	if err := RegisterTask(reg, taskSvc, projectSvc); err != nil {
		t.Fatalf("RegisterTask returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	rpcCtx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})
	raw, err := server.Handle(rpcCtx, []byte(`{
		"jsonrpc": "2.0",
		"id": "task-list",
		"method": "task.list",
		"params": { "projectId": "`+registered.ProjectID+`", "limit": 10 }
	}`))
	if err != nil {
		t.Fatalf("Handle task.list returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("task.list response error=%+v", resp.Error)
	}
	var listed taskrpc.TaskListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal task.list result: %v", err)
	}
	if len(listed.Tasks) != 1 {
		t.Fatalf("task count=%d want 1: %+v", len(listed.Tasks), listed.Tasks)
	}
	task := listed.Tasks[0]
	if task.AssignedToLabel != "Codex backend engineer" || task.CreatedByLabel != "Codex backend engineer" {
		t.Fatalf("task labels assigned=%q created=%q", task.AssignedToLabel, task.CreatedByLabel)
	}
}

func newRPCTaskStack(t *testing.T) (*projectapp.Service, *taskinfra.SQLiteRepository, *taskapp.Service) {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	projects, err := projectinfra.NewRepository(handle)
	if err != nil {
		t.Fatalf("new project repo: %v", err)
	}
	members, err := projectinfra.NewMemberRepository(handle)
	if err != nil {
		t.Fatalf("new member repo: %v", err)
	}
	workspaces, err := projectinfra.NewWorkspaceRepository(handle)
	if err != nil {
		t.Fatalf("new workspace repo: %v", err)
	}
	projectSvc, err := projectapp.NewService(projectapp.Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		LinkTokens: rpcProjectLinkTokenIssuer{},
		Caller:     caller.ContextResolver{},
		Configs:    rpcProjectConfigValidator{},
		Events:     rpcProjectEventPublisher{},
	})
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	taskRepo, err := taskinfra.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("new task repo: %v", err)
	}
	taskSvc, err := taskapp.NewService(taskRepo, taskdomain.FixedClock{T: time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)}, caller.ContextResolver{}, projectSvc, rpcTaskProjectLoader{projectSvc}, slog.Default())
	if err != nil {
		t.Fatalf("new task service: %v", err)
	}
	return projectSvc, taskRepo, taskSvc
}

type rpcTaskProjectLoader struct {
	projects *projectapp.Service
}

func (l rpcTaskProjectLoader) Get(ctx context.Context, projectID types.ProjectID) (project.Project, error) {
	return l.projects.GetProject(ctx, projectID)
}
