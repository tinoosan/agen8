package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectinfra "github.com/tinoosan/agen8-mcp-server/internal/services/project/infra"
	projectrpc "github.com/tinoosan/agen8-mcp-server/internal/services/project/rpc"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRegisterProjectDispatchSaveListAndClusterSpace(t *testing.T) {
	projectID := projectapp.ProjectIDForRoot("/tmp/project-1")
	svc := newRPCProjectService(t, []spacedomain.SpaceRecord{
		{ID: "space-1", ProjectID: string(projectID), Title: "Alpha", Status: spacedomain.SpaceStatusOpen},
	})
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
	gotProjectID := saved.Project.ID

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.cluster.save",
		"params": {
			"projectId": "`+gotProjectID+`",
			"clusterId": "cluster-1",
			"name": "Launch"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle project.cluster.save returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.cluster.save response error=%+v", resp.Error)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "3",
		"method": "project.cluster.space.save",
		"params": {
			"projectId": "`+gotProjectID+`",
			"clusterId": "cluster-1",
			"spaceId": "space-1",
			"sortOrder": 3,
			"pinned": true
		}
	}`))
	if err != nil {
		t.Fatalf("Handle project.cluster.space.save returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.cluster.space.save response error=%+v", resp.Error)
	}
	var clusterSpace projectrpc.ClusterSpaceSaveResult
	if err := json.Unmarshal(resp.Result, &clusterSpace); err != nil {
		t.Fatalf("unmarshal project.cluster.space.save result: %v", err)
	}
	if clusterSpace.Space.SpaceID != "space-1" || !clusterSpace.Space.Pinned || clusterSpace.Space.SortOrder != 3 {
		t.Fatalf("cluster space result=%+v", clusterSpace.Space)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "4",
		"method": "project.space.list",
		"params": { "projectId": "`+gotProjectID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.space.list returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.space.list response error=%+v", resp.Error)
	}
	var spaces projectrpc.ProjectSpaceListResult
	if err := json.Unmarshal(resp.Result, &spaces); err != nil {
		t.Fatalf("unmarshal project.space.list result: %v", err)
	}
	if len(spaces.Spaces) != 1 || spaces.Spaces[0].SpaceID != "space-1" || !spaces.Spaces[0].Pinned {
		t.Fatalf("project.space.list result=%+v", spaces.Spaces)
	}
}

func TestRegisterProjectMapsInvalidParams(t *testing.T) {
	svc := newRPCProjectService(t, nil)
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
	svc := newRPCProjectService(t, nil)
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

func newRPCProjectService(t *testing.T, spaces []spacedomain.SpaceRecord) *projectapp.Service {
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
	clusters, err := projectinfra.NewSQLiteClusterRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteClusterRepository: %v", err)
	}
	svc, err := projectapp.NewService(projectapp.Config{
		Projects: projects,
		Clusters: clusters,
		Spaces:   newRPCProjectSpaceLoader(spaces),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type rpcProjectSpaceLoader struct {
	spaces map[spacedomain.SpaceID]spacedomain.SpaceRecord
}

func newRPCProjectSpaceLoader(spaces []spacedomain.SpaceRecord) *rpcProjectSpaceLoader {
	loader := &rpcProjectSpaceLoader{spaces: map[spacedomain.SpaceID]spacedomain.SpaceRecord{}}
	for _, space := range spaces {
		loader.spaces[space.ID] = space
	}
	return loader
}

func (l *rpcProjectSpaceLoader) Get(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := l.spaces[id]
	if !ok {
		return spacedomain.SpaceRecord{}, fmt.Errorf("space %s not found", id)
	}
	return space, nil
}

func (l *rpcProjectSpaceLoader) List(_ context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
	out := make([]spacedomain.SpaceRecord, 0, len(l.spaces))
	for _, space := range l.spaces {
		if filter.ProjectID != "" && string(space.ProjectID) != filter.ProjectID {
			continue
		}
		out = append(out, space)
	}
	return out, nil
}

func (l *rpcProjectSpaceLoader) ListMembers(context.Context, member.Filter) ([]member.Record, error) {
	return nil, nil
}
