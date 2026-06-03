package domain

import "fmt"

type Registry struct {
	runtimes map[string]Runtime
}

func NewRegistry(runtimes ...Runtime) (*Registry, error) {
	out := &Registry{runtimes: map[string]Runtime{}}
	for _, rt := range runtimes {
		if rt == nil {
			continue
		}
		kind := NormalizeRuntimeKind(rt.Kind())
		if kind == "" {
			return nil, fmt.Errorf("runtime kind is required")
		}
		if kind == RuntimeKindInternal {
			return nil, fmt.Errorf("runtime kind %q is reserved", RuntimeKindInternal)
		}
		if _, exists := out.runtimes[kind]; exists {
			return nil, fmt.Errorf("duplicate runtime kind %q", kind)
		}
		out.runtimes[kind] = rt
	}
	return out, nil
}

func (r *Registry) Get(kind string) (Runtime, error) {
	normalized := NormalizeRuntimeKind(kind)
	if normalized == "" || normalized == RuntimeKindInternal {
		return nil, nil
	}
	if r == nil {
		return nil, fmt.Errorf("runtime kind %q is not supported", normalized)
	}
	rt, ok := r.runtimes[normalized]
	if !ok {
		return nil, fmt.Errorf("runtime kind %q is not supported", normalized)
	}
	return rt, nil
}
