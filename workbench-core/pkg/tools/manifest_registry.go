package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type ToolManifestRegistry interface {
	ListToolIDs(ctx context.Context) ([]ToolID, error)
	GetManifest(ctx context.Context, toolID ToolID) (manifestJSON []byte, ok bool, err error)
}

type CompositeToolManifestRegistry struct {
	Providers []ToolManifestRegistry
	Logf      func(format string, args ...any)
}

func NewCompositeToolManifestRegistry(providers ...ToolManifestRegistry) *CompositeToolManifestRegistry {
	return &CompositeToolManifestRegistry{Providers: providers}
}

func (r *CompositeToolManifestRegistry) ListToolIDs(ctx context.Context) ([]ToolID, error) {
	seen := make(map[ToolID]struct{})
	out := make([]ToolID, 0)

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

func (r *CompositeToolManifestRegistry) GetManifest(ctx context.Context, toolID ToolID) ([]byte, bool, error) {
	for _, p := range r.Providers {
		if p == nil {
			continue
		}
		b, ok, err := p.GetManifest(ctx, toolID)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return b, true, nil
		}
	}
	return nil, false, nil
}

type DiskManifestProvider struct {
	BaseDir string
	Logf    func(format string, args ...any)
}

func NewDiskManifestProvider(baseDir string) *DiskManifestProvider {
	return &DiskManifestProvider{BaseDir: baseDir}
}

func (p *DiskManifestProvider) ListToolIDs(_ context.Context) ([]ToolID, error) {
	if p == nil || p.BaseDir == "" {
		return []ToolID{}, nil
	}
	des, err := os.ReadDir(p.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ToolID{}, nil
		}
		return nil, err
	}

	var out []ToolID
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		id, err := ParseToolID(de.Name())
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

func (p *DiskManifestProvider) GetManifest(_ context.Context, toolID ToolID) ([]byte, bool, error) {
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
	if _, err := ParseUserToolManifest(b); err != nil {
		return nil, false, fmt.Errorf("invalid manifest.json for %q: %w", toolID.String(), err)
	}
	return b, true, nil
}

type StaticManifestProvider struct {
	Manifests map[ToolID][]byte
}

func (p StaticManifestProvider) ListToolIDs(_ context.Context) ([]ToolID, error) {
	out := make([]ToolID, 0, len(p.Manifests))
	for id := range p.Manifests {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

func (p StaticManifestProvider) GetManifest(_ context.Context, toolID ToolID) ([]byte, bool, error) {
	b, ok := p.Manifests[toolID]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), b...), true, nil
}
