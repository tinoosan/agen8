package mcp

import (
	"sort"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
)

type Registry struct {
	defs  []toolDef
	names map[string]struct{}
}

func NewRegistry() (*Registry, error) {
	defs, err := buildToolDefs()
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		if def.native.internal {
			continue
		}
		name := strings.TrimSpace(def.name())
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}
	return &Registry{defs: defs, names: names}, nil
}

func EnabledToolNames() ([]string, error) {
	defs, err := buildToolDefs()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.name())
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (r *Registry) Defs() []toolDef {
	if r == nil {
		return nil
	}
	return r.defs
}

func (r *Registry) Has(name string) bool {
	if r == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, ok := r.names[name]
	return ok
}

func (r *Registry) Catalog() types.ToolDiscoveryCatalog {
	return buildToolDiscoveryCatalog(r.defs)
}
