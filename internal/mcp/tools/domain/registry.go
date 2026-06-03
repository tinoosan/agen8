package domain

import (
	"context"
	"encoding/json"
)

// Registry defines the contract for a unified tool registry.
type Registry interface {
	AddSource(source ToolSource) error
	RemoveSource(sourceID string) error
	RefreshSource(sourceID string) error
	Sources() []ToolSource
	Get(name string) (Tool, bool)
	List() []Tool
	ListBySource(sourceID string) []Tool
	Definitions() []ToolDefinition
	Catalog() []CatalogEntry
	MemberCatalog(policy RolePolicy) MemberToolCatalog
	Dispatch(ctx context.Context, name string, args json.RawMessage) (ToolResult, error)
	DefinitionsForRole(policy RolePolicy) []ToolDefinition
	BridgeToolNames(policy RolePolicy) []string
}
