package app

import (
	"context"
	"sync"

	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type MemoryWakeRegistry struct {
	mu      sync.Mutex
	waiters map[domain.RequestID][]chan domain.Request
}

func NewMemoryWakeRegistry() *MemoryWakeRegistry {
	return &MemoryWakeRegistry{waiters: map[domain.RequestID][]chan domain.Request{}}
}

func (r *MemoryWakeRegistry) Wait(ctx context.Context, id domain.RequestID) (domain.Request, error) {
	ch := make(chan domain.Request, 1)
	r.mu.Lock()
	r.waiters[id] = append(r.waiters[id], ch)
	r.mu.Unlock()
	select {
	case req := <-ch:
		return req, nil
	case <-ctx.Done():
		r.remove(id, ch)
		return domain.Request{}, ctx.Err()
	}
}

func (r *MemoryWakeRegistry) Notify(req domain.Request) {
	r.mu.Lock()
	waiters := r.waiters[req.ID]
	delete(r.waiters, req.ID)
	r.mu.Unlock()
	for _, ch := range waiters {
		ch <- req
		close(ch)
	}
}

func (r *MemoryWakeRegistry) remove(id domain.RequestID, ch chan domain.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiters := r.waiters[id]
	for i, candidate := range waiters {
		if candidate == ch {
			r.waiters[id] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(r.waiters[id]) == 0 {
		delete(r.waiters, id)
	}
}
