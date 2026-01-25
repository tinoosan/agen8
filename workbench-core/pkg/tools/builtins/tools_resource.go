package builtins

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

// ToolsResource exposes tool discovery under the VFS mount "/tools".
type ToolsResource struct {
	BaseDir string
	Mount   string
	Registry pkgtools.ToolManifestRegistry
}

func NewToolsResource(reg pkgtools.ToolManifestRegistry) (*ToolsResource, error) {
	if reg == nil {
		return nil, fmt.Errorf("tool manifest registry is required")
	}
	return &ToolsResource{
		BaseDir:  "",
		Mount:    vfs.MountTools,
		Registry: reg,
	}, nil
}

func (tr *ToolsResource) List(subpath string) ([]vfs.Entry, error) {
	if tr == nil || tr.Registry == nil {
		return nil, fmt.Errorf("tool registry not configured")
	}
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean != "" && clean != "." {
		return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", clean)
	}

	ids, err := tr.Registry.ListToolIDs(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]vfs.Entry, 0, len(ids))
	for _, id := range ids {
		out = append(out, vfs.Entry{Path: id.String(), IsDir: true})
	}
	return out, nil
}

func (tr *ToolsResource) Read(subpath string) ([]byte, error) {
	if tr == nil || tr.Registry == nil {
		return nil, fmt.Errorf("tool registry not configured")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, fmt.Errorf("tools read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("tools read: path required (try '<toolId>')")
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("tools read: path required (try '<toolId>')")
	}
	if len(parts) == 1 {
		return tr.readManifest(parts[0])
	}
	if len(parts) == 2 && parts[1] == "manifest.json" {
		return tr.readManifest(parts[0])
	}
	return nil, fmt.Errorf("tools read: nested paths are not allowed (got %q)", clean)
}

func (tr *ToolsResource) readManifest(toolID string) ([]byte, error) {
	id, err := pkgtools.ParseToolID(toolID)
	if err != nil {
		return nil, fmt.Errorf("tools read: invalid toolId %q", toolID)
	}
	b, ok, err := tr.Registry.GetManifest(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("tools read: tool not found %q", id.String())
	}
	return b, nil
}

func (tr *ToolsResource) Write(_ string, _ []byte) error {
	return fmt.Errorf("tools write: not supported (tools is read-only)")
}

func (tr *ToolsResource) Append(_ string, _ []byte) error {
	return fmt.Errorf("tools append: not supported (tools is read-only)")
}
