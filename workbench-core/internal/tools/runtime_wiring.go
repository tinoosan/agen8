package tools

import (
	"fmt"

	"github.com/tinoosan/workbench-core/internal/vfs"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

// RuntimeWiring bundles discovery and invocation wiring for tools.
type RuntimeWiring struct {
	Resource vfs.Resource
	Registry pkgtools.ToolRegistry
}

// NewRuntimeWiring constructs a tools resource and runner registry from a manifest registry.
func NewRuntimeWiring(manifests pkgtools.ToolManifestRegistry, invokers pkgtools.ToolRegistry) (*RuntimeWiring, error) {
	if manifests == nil {
		return nil, fmt.Errorf("tool manifest registry is required")
	}
	if invokers == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	res, err := NewToolsResource(manifests)
	if err != nil {
		return nil, err
	}
	return &RuntimeWiring{
		Resource: res,
		Registry: invokers,
	}, nil
}
