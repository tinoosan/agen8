package tools

import (
	"context"
	"fmt"
	"sort"

	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

// BuiltinManifestProvider exposes builtin manifests registered via init() into this package.
type BuiltinManifestProvider struct {
	manifests map[pkgtools.ToolID][]byte
}

func NewBuiltinManifestProvider() (*BuiltinManifestProvider, error) {
	m := make(map[pkgtools.ToolID][]byte, len(builtinDefs))
	for _, def := range builtinDefs {
		manifest, err := pkgtools.ParseBuiltinToolManifest(def.Manifest)
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

func (p *BuiltinManifestProvider) ListToolIDs(_ context.Context) ([]pkgtools.ToolID, error) {
	ids := make([]pkgtools.ToolID, 0, len(p.manifests))
	for id := range p.manifests {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return ids, nil
}

func (p *BuiltinManifestProvider) GetManifest(_ context.Context, toolID pkgtools.ToolID) ([]byte, bool, error) {
	if p == nil || p.manifests == nil {
		return nil, false, nil
	}
	b, ok := p.manifests[toolID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}
