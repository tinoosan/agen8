package rpc

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/services/toolpolicy/app"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

func newTestHandler() *Handler {
	return NewHandler(app.NewService())
}

func TestHandler_Authorize(t *testing.T) {
	h := newTestHandler()
	result, err := h.Authorize(context.Background(), protocol.ToolpolicyAuthorizeParams{
		SpaceID:      "space-1",
		MemberType:   "coordinator",
		MemberCount:  3,
		HasReviewer:  true,
		AllowedTools: []string{"code_exec", "shell_exec"},
	})
	if err != nil {
		t.Fatal(err)
	}
	toolSet := map[string]bool{}
	for _, tool := range result.Allowed {
		toolSet[tool] = true
	}
	if !toolSet["space"] {
		t.Error("coordinator should have space merged in")
	}
}

func TestHandler_SystemTools(t *testing.T) {
	h := newTestHandler()
	result, err := h.SystemTools(context.Background(), protocol.ToolpolicySystemToolsParams{
		MemberType:  "reviewer",
		MemberCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolSet := map[string]bool{}
	for _, tool := range result.Tools {
		toolSet[tool] = true
	}
	if !toolSet["task"] {
		t.Error("reviewer should have task in system tools")
	}
	if !toolSet["space"] {
		t.Error("reviewer should have space in system tools")
	}
}

func TestHandler_Defaults(t *testing.T) {
	h := newTestHandler()
	result, err := h.Defaults(context.Background(), protocol.ToolpolicyDefaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.WorkerTools) == 0 {
		t.Error("expected non-empty worker tools")
	}
	if len(result.CoordinatorBase) == 0 {
		t.Error("expected non-empty coordinator base tools")
	}
	if result.CoordinatorWithWorkers == nil {
		t.Error("expected coordinator space tools slice to be initialized")
	}
}
