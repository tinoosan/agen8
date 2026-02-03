package hosttools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestBrowserTool_Execute_Start(t *testing.T) {
	tool := &BrowserTool{}
	req, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"start"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpBrowser {
		t.Fatalf("expected op %q, got %q", types.HostOpBrowser, req.Op)
	}
	if string(req.Input) != `{"action":"start"}` {
		t.Fatalf("expected raw input to be preserved, got %s", string(req.Input))
	}
}

func TestBrowserTool_Execute_RequiresSessionForNavigate(t *testing.T) {
	tool := &BrowserTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"navigate","url":"https://example.com"}`))
	if err == nil {
		t.Fatalf("expected error")
	}
}
