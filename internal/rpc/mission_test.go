package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	missioninfra "github.com/tinoosan/agen8/internal/services/mission/infra"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

var rpcMissionTestNow = time.Date(2026, 5, 26, 9, 0, 0, 0, time.UTC)

func TestRegisterMissionDispatchCreate(t *testing.T) {
	svc := newRPCMissionService(t)
	reg := NewRegistry()
	if err := RegisterMission(reg, svc); err != nil {
		t.Fatalf("RegisterMission returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "mission.create",
		"params": {
			"projectId": "project-1",
			"title": "Ship the mission service",
			"description": "rebuild the mission service boundary"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result struct {
		Mission struct {
			ID          string `json:"id"`
			ProjectID   string `json:"projectId"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Status      string `json:"status"`
		} `json:"mission"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Mission.ID == "" || result.Mission.ProjectID != "project-1" || result.Mission.Title != "Ship the mission service" || result.Mission.Status != "draft" {
		t.Fatalf("mission result=%+v", result.Mission)
	}
}

func TestRegisterMissionRequiresIdentity(t *testing.T) {
	svc := newRPCMissionService(t)
	reg := NewRegistry()
	if err := RegisterMission(reg, svc); err != nil {
		t.Fatalf("RegisterMission returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "mission.get",
		"params": { "missionId": "mission-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request", resp.Error)
	}
}

func TestRegisterMissionMapsInvalidParams(t *testing.T) {
	svc := newRPCMissionService(t)
	reg := NewRegistry()
	if err := RegisterMission(reg, svc); err != nil {
		t.Fatalf("RegisterMission returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "mission.create",
		"params": { "projectId": "project-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func TestRegisterMissionDispatchesFullCRUDSurface(t *testing.T) {
	svc := newRPCMissionService(t)
	reg := NewRegistry()
	if err := RegisterMission(reg, svc); err != nil {
		t.Fatalf("RegisterMission returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})
	call := func(method string, params map[string]any) json.RawMessage {
		t.Helper()
		req := map[string]any{
			"jsonrpc": "2.0",
			"id":      method,
			"method":  method,
			"params":  params,
		}
		rawReq, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal request for %s: %v", method, err)
		}
		rawResp, err := server.Handle(ctx, rawReq)
		if err != nil {
			t.Fatalf("%s Handle returned error: %v", method, err)
		}
		resp := decodeRPCResponse(t, rawResp)
		if resp.Error != nil {
			t.Fatalf("%s response error=%+v", method, resp.Error)
		}
		return resp.Result
	}

	createResult := struct {
		Mission struct {
			ID string `json:"id"`
		} `json:"mission"`
	}{}
	if err := json.Unmarshal(call(MethodMissionCreate, map[string]any{
		"projectId":   "project-1",
		"title":       "Reachable mission RPC",
		"description": "verify every registered method",
		"startDate":   "2026-06-01",
		"endDate":     "2026-06-30",
	}), &createResult); err != nil {
		t.Fatalf("unmarshal mission.create result: %v", err)
	}
	missionID := createResult.Mission.ID
	if missionID == "" {
		t.Fatal("mission id is empty")
	}

	call(MethodMissionGet, map[string]any{"missionId": missionID})
	call(MethodMissionList, map[string]any{"projectId": "project-1"})
	call(MethodMissionList, map[string]any{"projectId": "project-1", "status": []string{"draft"}})
	call(MethodMissionUpdate, map[string]any{"missionId": missionID, "title": "Updated mission"})
	call(MethodMissionProgress, map[string]any{"missionId": missionID})
	call(MethodMissionHistory, map[string]any{"missionId": missionID})

	krCreateResult := struct {
		KeyResult struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"keyResult"`
	}{}
	if err := json.Unmarshal(call(MethodMissionKRCreate, map[string]any{
		"missionId":       missionID,
		"title":           "Activation KR",
		"measurementType": "number",
		"direction":       "increase",
		"targetValue":     100,
	}), &krCreateResult); err != nil {
		t.Fatalf("unmarshal mission.kr.create result: %v", err)
	}
	keyResultID := krCreateResult.KeyResult.ID
	if keyResultID == "" {
		t.Fatal("key result id is empty")
	}

	call(MethodMissionKRGet, map[string]any{"keyResultId": keyResultID})
	call(MethodMissionKRList, map[string]any{"missionId": missionID})
	updateResult := struct {
		KeyResult struct {
			Version int64 `json:"version"`
		} `json:"keyResult"`
	}{}
	if err := json.Unmarshal(call(MethodMissionKRUpdate, map[string]any{
		"keyResultId": keyResultID,
		"title":       "Updated KR",
	}), &updateResult); err != nil {
		t.Fatalf("unmarshal mission.kr.update result: %v", err)
	}
	call(MethodMissionUpdate, map[string]any{"missionId": missionID, "status": "active"})

	progressResult := struct {
		KeyResult struct {
			Version int64 `json:"version"`
		} `json:"keyResult"`
	}{}
	if err := json.Unmarshal(call(MethodMissionKRProgress, map[string]any{
		"keyResultId":     keyResultID,
		"value":           50,
		"note":            "halfway",
		"expectedVersion": updateResult.KeyResult.Version,
	}), &progressResult); err != nil {
		t.Fatalf("unmarshal mission.kr.progress result: %v", err)
	}
	call(MethodMissionKRHistory, map[string]any{"keyResultId": keyResultID})
	if err := json.Unmarshal(call(MethodMissionKRProgress, map[string]any{
		"keyResultId":     keyResultID,
		"value":           100,
		"note":            "done",
		"expectedVersion": progressResult.KeyResult.Version,
	}), &progressResult); err != nil {
		t.Fatalf("unmarshal second mission.kr.progress result: %v", err)
	}
	call(MethodMissionKRReopen, map[string]any{"keyResultId": keyResultID})
	call(MethodMissionKRDelete, map[string]any{"keyResultId": keyResultID})
	call(MethodMissionDelete, map[string]any{"missionId": missionID})
}

func TestRegisterMissionActivationPreconditionIsInvalidParams(t *testing.T) {
	svc := newRPCMissionService(t)
	reg := NewRegistry()
	if err := RegisterMission(reg, svc); err != nil {
		t.Fatalf("RegisterMission returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{
		UserID:   "user-1",
		MemberID: "member-coordinator",
	})
	request := func(method string, params map[string]any) Response {
		t.Helper()
		rawReq, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      method,
			"method":  method,
			"params":  params,
		})
		if err != nil {
			t.Fatalf("marshal request for %s: %v", method, err)
		}
		rawResp, err := server.Handle(ctx, rawReq)
		if err != nil {
			t.Fatalf("%s Handle returned error: %v", method, err)
		}
		return decodeRPCResponse(t, rawResp)
	}
	// Activation requires at least one non-dropped key result. A mission with no
	// key results must therefore fail the precondition. (The former KR->owner
	// project requirement was removed, so a mission with a KR now activates
	// cleanly — this test deliberately creates none to exercise the gate.)
	createResp := request(MethodMissionCreate, map[string]any{
		"projectId": "project-1",
		"title":     "Needs a key result",
	})
	var createResult struct {
		Mission struct {
			ID string `json:"id"`
		} `json:"mission"`
	}
	if err := json.Unmarshal(createResp.Result, &createResult); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}

	resp := request(MethodMissionUpdate, map[string]any{
		"missionId": createResult.Mission.ID,
		"status":    "active",
	})
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("activation error=%+v want invalid params", resp.Error)
	}
	if resp.Error.Message == "internal error" {
		t.Fatalf("activation error should expose precondition, got %q", resp.Error.Message)
	}
}

func newRPCMissionService(t *testing.T) *missionapp.Service {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			return nil
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := missioninfra.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	svc, err := missionapp.NewService(
		repo,
		repo,
		repo,
		repo,
		rpcMissionClock{},
		caller.ContextResolver{},
		rpcMissionProjectLoader{},
		rpcMissionTaskLoader{},
		rpcMissionLinkedTaskLoader{},
		&rpcMissionEventPublisher{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type rpcMissionClock struct{}

func (rpcMissionClock) Now() time.Time { return rpcMissionTestNow }

type rpcMissionProjectLoader struct{}

func (rpcMissionProjectLoader) Get(_ context.Context, projectID types.ProjectID) (projectdomain.Project, error) {
	if projectID == "" {
		return projectdomain.Project{}, fmt.Errorf("project id is required")
	}
	return projectdomain.New(projectdomain.NewInput{
		ID:        projectID,
		Root:      "/tmp/" + string(projectID),
		Title:     "Research",
		Status:    projectdomain.StatusOpen,
		CreatedAt: rpcMissionTestNow,
		UpdatedAt: rpcMissionTestNow,
	})
}

type rpcMissionTaskLoader struct{}

func (rpcMissionTaskLoader) Get(_ context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error) {
	if taskID == "" {
		return taskdomain.Task{}, fmt.Errorf("task id is required")
	}
	return taskdomain.Task{ID: taskID, Status: taskdomain.TaskStatusSucceeded}, nil
}

type rpcMissionLinkedTaskLoader struct{}

func (rpcMissionLinkedTaskLoader) ListTaskIDsForKeyResult(context.Context, krdomain.KeyResultID) ([]taskdomain.TaskID, error) {
	return nil, nil
}

type rpcMissionEventPublisher struct{}

func (*rpcMissionEventPublisher) Append(context.Context, types.EventRecord) error {
	return nil
}

var _ missionapp.ProjectLoader = rpcMissionProjectLoader{}
var _ missionapp.TaskLoader = rpcMissionTaskLoader{}
var _ missionapp.LinkedTaskLoader = rpcMissionLinkedTaskLoader{}
var _ missionapp.EventPublisher = (*rpcMissionEventPublisher)(nil)
var _ missiondomain.Clock = rpcMissionClock{}
