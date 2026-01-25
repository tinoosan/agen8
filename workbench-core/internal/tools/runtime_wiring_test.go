package tools

import (
	"context"
	"encoding/json"
	"testing"

	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

type noopInvoker struct{}

func (noopInvoker) Invoke(_ context.Context, _ pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	return pkgtools.ToolCallResult{Output: json.RawMessage(`{}`)}, nil
}

func TestNewRuntimeWiring_ReturnsResourceAndRegistry(t *testing.T) {
	manifestBytes := []byte(`{"id":"custom.echo"}`)
	reg := pkgtools.StaticManifestProvider{
		Manifests: map[pkgtools.ToolID][]byte{
			pkgtools.ToolID("custom.echo"): manifestBytes,
		},
	}
	invokers := pkgtools.MapRegistry{
		pkgtools.ToolID("custom.echo"): noopInvoker{},
	}

	wiring, err := NewRuntimeWiring(reg, invokers)
	if err != nil {
		t.Fatalf("NewRuntimeWiring: %v", err)
	}
	if wiring.Resource == nil {
		t.Fatalf("expected resource to be set")
	}
	if _, ok := wiring.Registry.Get(pkgtools.ToolID("custom.echo")); !ok {
		t.Fatalf("expected invoker registry to contain tool")
	}

	entries, err := wiring.Resource.List("")
	if err != nil {
		t.Fatalf("resource list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 tool entry, got %d", len(entries))
	}
	if entries[0].Path != "custom.echo" {
		t.Fatalf("expected tool entry for custom.echo, got %q", entries[0].Path)
	}
}
