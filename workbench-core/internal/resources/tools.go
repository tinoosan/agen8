package resources

import "github.com/tinoosan/workbench-core/internal/tools"

// ToolsResource is a compatibility alias for tools.ToolsResource.
type ToolsResource = tools.ToolsResource

func NewToolsResource(reg tools.ToolManifestRegistry) (*ToolsResource, error) {
	return tools.NewToolsResource(reg)
}
