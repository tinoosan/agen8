package rpc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	projectrpc "github.com/tinoosan/agen8-mcp-server/internal/services/project/rpc"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRegisterProjectDispatchCreateAndList(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": {
			"root": "/tmp/project-1",
			"title": "Project One"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var saved projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &saved); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}
	if saved.Project.ID == "" || saved.Project.Status != "open" {
		t.Fatalf("project.create result=%+v", saved.Project)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.list",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle project.list returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.list response error=%+v", resp.Error)
	}
	var listed projectrpc.ProjectListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal project.list result: %v", err)
	}
	if len(listed.Projects) != 1 || listed.Projects[0].ID != saved.Project.ID {
		t.Fatalf("project.list result=%+v", listed.Projects)
	}
}

func TestRegisterProjectMapsInvalidParams(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.get",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func TestRegisterProjectArchiveThenDelete(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": { "root": "/tmp/archive-me", "title": "Archive Me" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.delete",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.delete returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error == nil {
		t.Fatalf("expected delete before archive to fail")
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "3",
		"method": "project.archive",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.archive returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.archive response error=%+v", resp.Error)
	}
	var archived projectrpc.ProjectArchiveResult
	if err := json.Unmarshal(resp.Result, &archived); err != nil {
		t.Fatalf("unmarshal project.archive result: %v", err)
	}
	if archived.Project.Status != "archived" {
		t.Fatalf("archived project=%+v", archived.Project)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "4",
		"method": "project.delete",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.delete returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.delete response error=%+v", resp.Error)
	}
}

func newRPCProjectService(t *testing.T) *projectapp.Service {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	projects, err := projectinfra.NewSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	members, err := projectinfra.NewMemberSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewMemberSQLiteRepository: %v", err)
	}
	svc, err := projectapp.NewService(projectapp.Config{
		Projects: projects,
		Members:  members,
		Caller:   caller.ContextResolver{},
		Configs:  rpcProjectConfigValidator{},
		Events:   rpcProjectEventPublisher{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type rpcProjectConfigValidator struct{}

func (rpcProjectConfigValidator) ValidateConfig(string, string, string) error { return nil }

type rpcProjectEventPublisher struct{}

func (rpcProjectEventPublisher) Publish(string, any) error { return nil }
