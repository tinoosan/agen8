package tools

import (
	"context"
	"encoding/json"

	"github.com/tinoosan/workbench-core/internal/types"
)

var demoStockManifest = []byte(`{"id":"github.com.acme.stock","version":"0.1.0","kind":"builtin","displayName":"Acme Stock","description":"Example builtin tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote for a symbol","inputSchema":{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]},"outputSchema":{"type":"object","properties":{"ok":{"type":"boolean"},"price":{"type":"number"}},"required":["ok","price"]}}]}`)

var demoDupeManifest = []byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"builtin","displayName":"Dupe Tool (builtin)","description":"Builtin should override disk","actions":[{"id":"dupe.noop","displayName":"No-op","description":"Returns an empty object","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("github.com.acme.stock"),
		Manifest: demoStockManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			return demoStockInvoker{}
		},
	})

	// This builtin is used to demonstrate that builtins override disk manifests in /tools.
	// It is intentionally not executable yet (no invoker registered).
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("github.com.dupe.tool"),
		Manifest: demoDupeManifest,
	})
}

// demoStockInvoker is a tiny example builtin tool invoker used by cmd/workbench.
//
// It exists to exercise the full runner lifecycle:
//   - tool discovery via /tools manifest
//   - tool.run invocation via ToolRunner
//   - persistence under /results/<callId>/...
type demoStockInvoker struct{}

func (demoStockInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	_ = ctx

	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "quote.latest" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "unsupported action"}
	}

	return ToolCallResult{
		Output: json.RawMessage(`{"ok":true,"price":123.45}`),
		Artifacts: []ToolArtifactWrite{
			{Path: "quote.json", Bytes: []byte(`{"symbol":"AAPL","price":123.45}`), MediaType: "application/json"},
			{Path: "notes.md", Bytes: []byte("# Quote\nAAPL = 123.45\n"), MediaType: "text/markdown"},
		},
	}, nil
}
