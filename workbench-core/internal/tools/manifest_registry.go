package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tinoosan/workbench-core/internal/types"
)

// ToolManifestRegistry is the single source of truth for tool discovery (/tools).
//
// Important: this registry is for *manifests* only.
//
// Execution still happens through tool.run and the runner's ToolRegistry/ToolInvoker:
//   - /tools is for discoverability (what tools exist, what actions exist)
//   - tool.run is for execution (how tools are invoked)
type ToolManifestRegistry interface {
	// ListToolIDs returns all discoverable tool IDs, sorted ascending by toolId string.
	//
	// Collisions are resolved by the registry implementation (e.g., builtin overrides disk).
	ListToolIDs(ctx context.Context) ([]types.ToolID, error)

	// GetManifest returns the manifest JSON bytes for toolID.
	//
	// ok=false means "tool not found" in this registry.
	GetManifest(ctx context.Context, toolID types.ToolID) (manifestJSON []byte, ok bool, err error)
}

// CompositeToolManifestRegistry merges multiple manifest registries with precedence.
//
// Each entry in Providers acts as a "provider" (builtin, disk, remote, etc).
// Individual providers do not implement precedence rules; this composite does.
//
// Provider precedence:
//   - providers are consulted in order
//   - first provider to claim a tool ID wins (builtin should be first)
//
// This is what makes /tools virtualization work: disk tools become "just another provider",
// and /tools does not rely on any physical directory layout.
type CompositeToolManifestRegistry struct {
	Providers []ToolManifestRegistry

	// Logf is optional; if set, non-fatal provider errors are logged and skipped.
	Logf func(format string, args ...any)
}

func NewCompositeToolManifestRegistry(providers ...ToolManifestRegistry) *CompositeToolManifestRegistry {
	return &CompositeToolManifestRegistry{Providers: providers}
}

func (r *CompositeToolManifestRegistry) ListToolIDs(ctx context.Context) ([]types.ToolID, error) {
	seen := make(map[types.ToolID]struct{})
	out := make([]types.ToolID, 0)

	for _, p := range r.Providers {
		if p == nil {
			continue
		}
		ids, err := p.ListToolIDs(ctx)
		if err != nil {
			if r.Logf != nil {
				r.Logf("tool manifest provider list error: %v", err)
			}
			continue
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func (r *CompositeToolManifestRegistry) GetManifest(ctx context.Context, toolID types.ToolID) ([]byte, bool, error) {
	for _, p := range r.Providers {
		if p == nil {
			continue
		}
		b, ok, err := p.GetManifest(ctx, toolID)
		if err != nil {
			// Errors are surfaced to callers; the host can decide whether to treat them as fatal.
			return nil, false, err
		}
		if ok {
			return b, true, nil
		}
	}
	return nil, false, nil
}

// BuiltinManifestProvider exposes builtin manifests registered via init() into this package.
//
// Builtins are validated (ParseBuiltinToolManifest) once, up-front, when the provider is created.
type BuiltinManifestProvider struct {
	manifests map[types.ToolID][]byte
}

func NewBuiltinManifestProvider() (*BuiltinManifestProvider, error) {
	m := make(map[types.ToolID][]byte, len(builtinDefs))
	for _, def := range builtinDefs {
		manifest, err := types.ParseBuiltinToolManifest(def.Manifest)
		if err != nil {
			return nil, fmt.Errorf("builtin manifest %q invalid: %w", def.ID.String(), err)
		}
		if manifest.ID != def.ID {
			return nil, fmt.Errorf("builtin manifest %q id mismatch (manifest id %q)", def.ID.String(), manifest.ID.String())
		}
		m[def.ID] = def.Manifest
	}
	return &BuiltinManifestProvider{manifests: m}, nil
}

func (p *BuiltinManifestProvider) ListToolIDs(_ context.Context) ([]types.ToolID, error) {
	ids := make([]types.ToolID, 0, len(p.manifests))
	for id := range p.manifests {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids, nil
}

func (p *BuiltinManifestProvider) GetManifest(_ context.Context, toolID types.ToolID) ([]byte, bool, error) {
	if p == nil || p.manifests == nil {
		return nil, false, nil
	}
	b, ok := p.manifests[toolID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}

// DiskManifestProvider exposes on-disk custom tool manifests as one ToolManifestRegistry provider.
//
// Disk structure (current compatibility):
//
//	<baseDir>/<toolId>/manifest.json
//
// Disk is optional: if baseDir is missing, it behaves like an empty provider.
type DiskManifestProvider struct {
	BaseDir string
	Logf    func(format string, args ...any)
}

func NewDiskManifestProvider(baseDir string) *DiskManifestProvider {
	return &DiskManifestProvider{BaseDir: baseDir}
}

func (p *DiskManifestProvider) ListToolIDs(_ context.Context) ([]types.ToolID, error) {
	if p == nil || p.BaseDir == "" {
		return []types.ToolID{}, nil
	}
	des, err := os.ReadDir(p.BaseDir)
	if err != nil {
		// Missing tools dir is fine; treat as empty.
		if os.IsNotExist(err) {
			return []types.ToolID{}, nil
		}
		return nil, err
	}

	var out []types.ToolID
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		id, err := types.ParseToolID(de.Name())
		if err != nil {
			continue
		}
		b, ok, err := p.GetManifest(context.Background(), id)
		if err != nil {
			if p.Logf != nil {
				p.Logf("disk tool %q skipped: %v", id.String(), err)
			}
			continue
		}
		if !ok {
			continue
		}
		// Defensive: ensure it is at least syntactically valid JSON.
		if !json.Valid(b) {
			if p.Logf != nil {
				p.Logf("disk tool %q skipped: manifest is not valid JSON", id.String())
			}
			continue
		}
		out = append(out, id)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func (p *DiskManifestProvider) GetManifest(_ context.Context, toolID types.ToolID) ([]byte, bool, error) {
	if p == nil || p.BaseDir == "" {
		return nil, false, nil
	}
	manifestPath := filepath.Join(p.BaseDir, toolID.String(), "manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	// Validate as a user manifest (builtin kind is reserved and rejected).
	if _, err := types.ParseUserToolManifest(b); err != nil {
		return nil, false, fmt.Errorf("invalid manifest.json for %q: %w", toolID.String(), err)
	}
	return b, true, nil
}

// StaticManifestProvider is a minimal in-memory manifest provider.
//
// This is primarily useful for tests: it lets you stand up a /tools mount without any
// on-disk directory layout.
type StaticManifestProvider struct {
	Manifests map[types.ToolID][]byte
}

func (p StaticManifestProvider) ListToolIDs(_ context.Context) ([]types.ToolID, error) {
	out := make([]types.ToolID, 0, len(p.Manifests))
	for id := range p.Manifests {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func (p StaticManifestProvider) GetManifest(_ context.Context, toolID types.ToolID) ([]byte, bool, error) {
	b, ok := p.Manifests[toolID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}
