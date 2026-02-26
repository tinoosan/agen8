package harness

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry stores available harness adapters keyed by adapter ID.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		if err := r.Register(adapter); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(adapter Adapter) error {
	if r == nil {
		return fmt.Errorf("harness registry is nil")
	}
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	id := normalizeAdapterID(adapter.ID())
	if id == "" {
		return fmt.Errorf("adapter id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[id]; exists {
		return fmt.Errorf("adapter %q already registered", id)
	}
	r.adapters[id] = adapter
	return nil
}

func (r *Registry) Get(id string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	id = normalizeAdapterID(id)
	if id == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[id]
	return adapter, ok
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func normalizeAdapterID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
