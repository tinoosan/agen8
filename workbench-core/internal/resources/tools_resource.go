package resources

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// BuiltinTool is the virtual (in-memory) representation of a tool.
type BuiltinTool struct {
	Manifest []byte // required: the tool manifest JSON
}

// ToolsResource exposes tool discovery and tool manifests under the VFS mount "/tools".
//
// VFS usage pattern (agent/host-facing)
//   - Discover tools:
//     fs.List("/tools") => entries like "/tools/<toolId>" (IsDir=true)
//   - Read a tool's manifest:
//     fs.Read("/tools/<toolId>") => manifest JSON bytes
//
// The agent does not need to know about "manifest.json" as a filename; reading a tool
// returns its manifest by default. Nested reads (e.g. "/tools/<toolId>/bin/x") are rejected.
//
// Storage model (implementation detail)
//   - Builtin tools live in memory (BuiltinRegistry).
//   - Custom tools live on disk under:
//     data/tools/<toolId>/manifest.json
//   - If the same toolId exists in both places, builtins win.
type ToolsResource struct {
	// BaseDir is the OS directory backing this resource (the sandbox root).
	// All operations are confined under BaseDir. The resource must reject any
	// subpath that would escape BaseDir (e.g. "..", absolute paths).
	//
	// BaseDir is an implementation detail; callers interact via virtual paths
	// like "/tools/<toolId>" through the VFS.
	BaseDir string

	// Mount is the virtual mount name used by the VFS.
	// Example: "tools" maps to the virtual namespace "/tools".
	Mount           string
	BuiltinRegistry map[string]BuiltinTool
}

func NewToolsResource() (*ToolsResource, error) {
	// Default to "data/tools". Tests/callers can override BaseDir if needed.
	baseDir := fsutil.GetToolsDir(config.DataDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating tools directory %s: %w", baseDir, err)
	}
	return &ToolsResource{
		BaseDir:         baseDir,
		Mount:           vfs.MountTools,
		BuiltinRegistry: make(map[string]BuiltinTool),
	}, nil
}

// List lists entries under subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
// List("") lists the resource root.
//
// Layout:
//   - Builtins live in-memory in BuiltinRegistry.
//   - Custom tools live on disk under BaseDir/<toolId>/manifest.json.
//
// List("") returns tool IDs as directories (IsDir=true), including builtins and
// disk tools, in stable sorted order. If a tool ID exists in both places,
// builtins win (it is still listed once).
func (tr *ToolsResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := normalizeAndSplitTools(subpath)
	if err != nil {
		return nil, fmt.Errorf("tools list: %w", err)
	}

	if clean != "" && clean != "." {
		return nil, fmt.Errorf("cannot list %q (allowed: '')", clean)
	}

	diskIDs, err := listDiskToolIDs(tr.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("tools list: %w", err)
	}

	seen := make(map[string]struct{}, len(tr.BuiltinRegistry)+len(diskIDs))
	all := make([]string, 0, len(tr.BuiltinRegistry)+len(diskIDs))

	for id := range tr.BuiltinRegistry {
		seen[id] = struct{}{}
		all = append(all, id)
	}
	for _, id := range diskIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		all = append(all, id)
	}

	sort.Strings(all)
	out := make([]vfs.Entry, 0, len(all))
	for _, id := range all {
		out = append(out, vfs.Entry{Path: id, IsDir: true})
	}
	return out, nil
}

// Read reads a file at subpath relative to BaseDir.
// subpath is resource-relative (no leading "/").
//
// Supported subpaths:
//   - "<toolId>" => manifest JSON bytes
//   - "<toolId>/manifest.json" => manifest JSON bytes
//
// Any other nested path is rejected.
// If a tool exists in both builtins and on disk, builtins win.
func (tr *ToolsResource) Read(subpath string) ([]byte, error) {
	clean, parts, err := normalizeAndSplitTools(subpath)
	if err != nil {
		return nil, fmt.Errorf("tools read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("tool ID required")
	}

	toolID := parts[0]
	if len(parts) == 1 {
		// ok: Read("<toolId>")
	} else if len(parts) == 2 && parts[1] == "manifest.json" {
		// ok: Read("<toolId>/manifest.json")
	} else {
		return nil, fmt.Errorf("cannot read %q (allowed: <toolId> or <toolId>/manifest.json)", clean)
	}

	if tool, ok := tr.BuiltinRegistry[toolID]; ok {
		if len(tool.Manifest) == 0 {
			return nil, fmt.Errorf("tool %q has no manifest", toolID)
		}
		return tool.Manifest, nil
	}

	manifestPath := toolManifestPath(tr.BaseDir, toolID)
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("tool %q not found", toolID)
		}
		return nil, fmt.Errorf("read tool %q manifest: %w", toolID, err)
	}
	return b, nil
}

// Write replaces the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (tr *ToolsResource) Write(subpath string, data []byte) error {
	return fmt.Errorf("write not supported for tools resource")
}

// Append appends bytes to the file at subpath (creating parent directories if needed).
// subpath is resource-relative (no leading "/").
func (tr *ToolsResource) Append(subpath string, data []byte) error {
	return fmt.Errorf("append not supported for tools resource")
}

func toolManifestPath(baseDir, toolID string) string {
	return filepath.Join(baseDir, toolID, "manifest.json")
}

func listDiskToolIDs(baseDir string) ([]string, error) {
	des, err := os.ReadDir(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}

	out := make([]string, 0)
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		toolID := de.Name()
		manifestPath := toolManifestPath(baseDir, toolID)
		info, err := os.Stat(manifestPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		out = append(out, toolID)
	}

	sort.Strings(out)
	return out, nil
}

// normalizeAndSplitTools enforces "resource-relative" subpaths and returns split parts.
//
// Treats "" and "." as root.
// Rejects absolute and escape attempts ("..", "a/../x").
func normalizeAndSplitTools(subpath string) (string, []string, error) {
	s := strings.TrimSpace(subpath)
	if s == "" || s == "." {
		return s, nil, nil
	}
	if strings.HasPrefix(s, "/") {
		return "", nil, fmt.Errorf("absolute paths not allowed: %q", subpath)
	}

	// Reject any explicit parent directory segments, even if they would clean away.
	// Example: "a/../x" is rejected.
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return "", nil, fmt.Errorf("invalid path: escapes mount root")
		}
	}

	// VFS subpaths always use "/" separators.
	clean := path.Clean(s)
	if clean == "." {
		return ".", nil, nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", nil, fmt.Errorf("invalid path: escapes mount root")
	}

	parts := strings.Split(clean, "/")
	for _, p := range parts {
		if p == "" {
			return "", nil, fmt.Errorf("invalid path: empty segment")
		}
	}
	return clean, parts, nil
}

// Example usage:
//
//	tools := &ToolsResource{
//		BaseDir:         tmpToolsDir, // e.g. t.TempDir()
//		Mount:           "tools",
//		BuiltinRegistry: map[string]BuiltinTool{},
//	}
//	tools.BuiltinRegistry["github.com.acme.stock"] = BuiltinTool{Manifest: []byte(`{"id":"github.com.acme.stock"}`)}
//	_ = os.MkdirAll(filepath.Join(tmpToolsDir, "github.com.other.stock"), 0755)
//	_ = os.WriteFile(filepath.Join(tmpToolsDir, "github.com.other.stock", "manifest.json"), []byte(`{"id":"github.com.other.stock"}`), 0644)
//
//	fs := vfs.NewFS()
//	fs.Mount("tools", tools)
//	_, _ = fs.List("/tools")                      // shows both tool IDs
//	_, _ = fs.Read("/tools/github.com.acme.stock") // returns builtin manifest bytes
