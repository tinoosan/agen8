package wake

import (
	"fmt"
	"sync"
)

type signalSubscription[F any] struct {
	filter F
	ch     chan struct{}
}

// SignalHub manages best-effort wake subscriptions keyed by arbitrary filter data.
type SignalHub[F any] struct {
	mu     sync.Mutex
	subs   map[string]signalSubscription[F]
	nextID uint64
}

func NewSignalHub[F any]() *SignalHub[F] {
	return &SignalHub[F]{subs: map[string]signalSubscription[F]{}}
}

func (h *SignalHub[F]) Subscribe(filter F) (<-chan struct{}, func()) {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch, func() {}
	}
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.nextID++
	id := fmt.Sprintf("wake-%d", h.nextID)
	h.subs[id] = signalSubscription[F]{filter: filter, ch: ch}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		sub, ok := h.subs[id]
		if ok {
			delete(h.subs, id)
		}
		h.mu.Unlock()
		if ok {
			close(sub.ch)
		}
	}
	return ch, cancel
}

func (h *SignalHub[F]) Notify(match func(F) bool) {
	if h == nil || match == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		if !match(sub.filter) {
			continue
		}
		select {
		case sub.ch <- struct{}{}:
		default:
		}
	}
}
