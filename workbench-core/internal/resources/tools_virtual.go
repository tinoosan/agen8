package resources

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// VirtualToolsResource exposes tool discovery under the VFS mount "/tools",
// backed by a tools.ToolManifestRegistry.
//
// Agent contract (unchanged)
//   - List("/tools") returns tool IDs as directory-like entries (IsDir=true).
//   - Read("/tools/<toolId>") returns that tool's manifest JSON bytes.
//   - Read("/tools/<toolId>/manifest.json") is also accepted for compatibility.
//
// /tools is read-only: Write and Append are not supported.
//
// This resource is intentionally *virtual*: it does not require an on-disk directory
// layout to exist. Disk tools (if any) are exposed via a registry provider, not by
// scanning a physical mount.
type VirtualToolsResource struct {
	// BaseDir is unused by this resource, but kept for consistency/debugging.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "tools" maps to the virtual namespace "/tools".
	Mount string

	Registry tools.ToolManifestRegistry
}

func NewVirtualToolsResource(reg tools.ToolManifestRegistry) (*VirtualToolsResource, error) {
	if reg == nil {
		return nil, fmt.Errorf("tool manifest registry is required")
	}
	return &VirtualToolsResource{
		BaseDir:  "",
		Mount:    vfs.MountTools,
		Registry: reg,
	}, nil
}

func (tr *VirtualToolsResource) List(subpath string) ([]vfs.Entry, error) {
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

func (tr *VirtualToolsResource) Read(subpath string) ([]byte, error) {
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

func (tr *VirtualToolsResource) readManifest(toolID string) ([]byte, error) {
	id, err := types.ParseToolID(toolID)
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

func (tr *VirtualToolsResource) Write(_ string, _ []byte) error {
	return fmt.Errorf("tools write: not supported (tools is read-only)")
}

func (tr *VirtualToolsResource) Append(_ string, _ []byte) error {
	return fmt.Errorf("tools append: not supported (tools is read-only)")
}
