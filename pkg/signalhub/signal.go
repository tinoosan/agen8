package signalhub

import (
	"fmt"
	"sync"
)

type subscription[F any] struct {
	filter F
	ch     chan struct{}
}

type payloadSubscription[F any, P any] struct {
	filter F
	ch     chan P
}

// Hub manages best-effort wake subscriptions keyed by arbitrary filter data.
type Hub[F any] struct {
	mu     sync.Mutex
	subs   map[string]subscription[F]
	nextID uint64
}

func New[F any]() *Hub[F] {
	return &Hub[F]{subs: map[string]subscription[F]{}}
}

func (h *Hub[F]) Subscribe(filter F) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.nextID++
	id := fmt.Sprintf("wake-%d", h.nextID)
	h.subs[id] = subscription[F]{filter: filter, ch: ch}
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

func (h *Hub[F]) Notify(match func(F) bool) {
	if match == nil {
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

func (h *Hub[F]) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// PayloadHub manages best-effort subscriptions that carry a typed payload.
type PayloadHub[F any, P any] struct {
	mu     sync.Mutex
	subs   map[string]payloadSubscription[F, P]
	nextID uint64
}

func NewPayload[F any, P any]() *PayloadHub[F, P] {
	return &PayloadHub[F, P]{subs: map[string]payloadSubscription[F, P]{}}
}

func (h *PayloadHub[F, P]) Subscribe(filter F) (<-chan P, func()) {
	if h == nil {
		ch := make(chan P)
		close(ch)
		return ch, func() {}
	}
	ch := make(chan P, 1)
	h.mu.Lock()
	h.nextID++
	id := fmt.Sprintf("wake-%d", h.nextID)
	h.subs[id] = payloadSubscription[F, P]{filter: filter, ch: ch}
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

func (h *PayloadHub[F, P]) Notify(payload P, match func(F) bool) {
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
		case sub.ch <- payload:
		default:
		}
	}
}

func (h *PayloadHub[F, P]) Len() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
