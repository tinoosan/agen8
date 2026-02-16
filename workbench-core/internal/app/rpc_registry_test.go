package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	implstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/store"
)

func TestRPCServer_Dispatch_UnknownMethod(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessStore := store.NewMemorySessionStore()
	srv := NewRPCServer(RPCServerConfig{Cfg: cfg, Session: newTestSessionService(cfg, sessStore)})

	id := "1"
	req := protocol.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "does.not.exist",
		Params:  json.RawMessage(`{}`),
	}
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil || resp.Error.Code != protocol.CodeMethodNotFound {
		t.Fatalf("expected method not found, got %+v", resp.Error)
	}
}

func TestRPCServer_Dispatch_MalformedParams(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "goal", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore := store.NewMemorySessionStore()
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := sessStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	srv := NewRPCServer(RPCServerConfig{Cfg: cfg, Run: run, Session: newTestSessionService(cfg, sessStore)})

	id := "1"
	req := protocol.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  protocol.MethodThreadGet,
		Params:  json.RawMessage(`[]`),
	}
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error == nil || resp.Error.Code != protocol.CodeInvalidParams || strings.TrimSpace(resp.Error.Message) != "invalid params" {
		t.Fatalf("expected invalid params, got %+v", resp.Error)
	}
}

func TestRPCServer_ThreadCreate_EmptyParamsAllowed(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sess, run, err := implstore.CreateSession(cfg, "goal", 8*1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessStore := store.NewMemorySessionStore()
	if err := sessStore.SaveSession(context.Background(), sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := sessStore.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	srv := NewRPCServer(RPCServerConfig{Cfg: cfg, Run: run, Session: newTestSessionService(cfg, sessStore)})

	id := "1"
	req := protocol.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  protocol.MethodThreadCreate,
	}
	resp := rpcRoundTrip(t, srv, req)
	if resp.Error != nil {
		t.Fatalf("thread.create error: %+v", resp.Error)
	}
	var out protocol.ThreadCreateResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got := strings.TrimSpace(string(out.Thread.ID)); got != run.SessionID {
		t.Fatalf("thread id=%q want %q", got, run.SessionID)
	}
}

func TestBuildMethodRegistry_DuplicateRegistrationFails(t *testing.T) {
	srv := &RPCServer{}
	_, err := buildMethodRegistry(srv, registerSessionHandlers, registerSessionHandlers)
	if err == nil {
		t.Fatalf("expected duplicate registration error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestRPCServer_MethodRegistry_CoversAllProtocolMethods(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessStore := store.NewMemorySessionStore()
	srv := NewRPCServer(RPCServerConfig{Cfg: cfg, Session: newTestSessionService(cfg, sessStore)})
	if srv.initErr != nil {
		t.Fatalf("unexpected init error: %v", srv.initErr)
	}

	expected := []string{
		protocol.MethodThreadCreate,
		protocol.MethodThreadGet,
		protocol.MethodTurnCreate,
		protocol.MethodTurnCancel,
		protocol.MethodItemList,
		protocol.MethodTaskList,
		protocol.MethodTaskClaim,
		protocol.MethodTaskCreate,
		protocol.MethodTaskComplete,
		protocol.MethodSessionStart,
		protocol.MethodSessionList,
		protocol.MethodSessionRename,
		protocol.MethodAgentList,
		protocol.MethodAgentStart,
		protocol.MethodAgentPause,
		protocol.MethodAgentResume,
		protocol.MethodSessionPause,
		protocol.MethodSessionResume,
		protocol.MethodSessionStop,
		protocol.MethodSessionDelete,
		protocol.MethodSessionGetTotals,
		protocol.MethodActivityList,
		protocol.MethodRunListChildren,
		protocol.MethodTeamGetStatus,
		protocol.MethodTeamGetManifest,
		protocol.MethodPlanGet,
		protocol.MethodModelList,
		protocol.MethodControlSetModel,
		protocol.MethodControlSetReasoning,
		protocol.MethodControlSetProfile,
		protocol.MethodArtifactList,
		protocol.MethodArtifactSearch,
		protocol.MethodArtifactGet,
		protocol.MethodEventsListPaginated,
		protocol.MethodEventsLatestSeq,
		protocol.MethodEventsCount,
	}

	if got, want := len(srv.methodHandlers), len(expected); got != want {
		t.Fatalf("method handler count=%d want=%d", got, want)
	}
	for _, method := range expected {
		if _, ok := srv.methodHandlers[method]; !ok {
			t.Fatalf("missing handler for %q", method)
		}
	}
}
