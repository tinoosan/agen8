package message

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/types"
)

type Service interface {
	state.MessageStore
	SubscribeWake(threadID, runID string) (<-chan struct{}, func())
}

type Manager struct {
	store state.MessageStore

	watchersMu sync.Mutex
	watchers   map[string]messageWakeWatcher
}

type messageWakeWatcher struct {
	threadID string
	runID    string
	ch       chan struct{}
}

func NewManager(store state.MessageStore) *Manager {
	return &Manager{
		store:    store,
		watchers: map[string]messageWakeWatcher{},
	}
}

func (m *Manager) SubscribeWake(threadID, runID string) (<-chan struct{}, func()) {
	if m == nil {
		ch := make(chan struct{})
		close(ch)
		return ch, func() {}
	}
	id := uuid.NewString()
	w := messageWakeWatcher{
		threadID: strings.TrimSpace(threadID),
		runID:    strings.TrimSpace(runID),
		ch:       make(chan struct{}, 1),
	}
	m.watchersMu.Lock()
	m.watchers[id] = w
	m.watchersMu.Unlock()
	cancel := func() {
		m.watchersMu.Lock()
		ww, ok := m.watchers[id]
		if ok {
			delete(m.watchers, id)
		}
		m.watchersMu.Unlock()
		if ok {
			close(ww.ch)
		}
	}
	return w.ch, cancel
}

func (m *Manager) notifyWake(threadID, runID string) {
	if m == nil {
		return
	}
	threadID = strings.TrimSpace(threadID)
	runID = strings.TrimSpace(runID)
	m.watchersMu.Lock()
	defer m.watchersMu.Unlock()
	for _, w := range m.watchers {
		if w.threadID != "" && w.threadID != threadID {
			continue
		}
		if w.runID != "" && w.runID != runID {
			continue
		}
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) PublishMessage(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	if m == nil || m.store == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	out, err := m.store.PublishMessage(ctx, msg)
	if err != nil {
		return types.AgentMessage{}, err
	}
	m.notifyWake(out.ThreadID, out.RunID)
	return out, nil
}

func (m *Manager) ClaimNextMessage(ctx context.Context, filter state.MessageClaimFilter, ttl time.Duration, consumerID string) (types.AgentMessage, error) {
	if m == nil || m.store == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	return m.store.ClaimNextMessage(ctx, filter, ttl, consumerID)
}

func (m *Manager) AckMessage(ctx context.Context, messageID string, result state.MessageAckResult) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("message store is not configured")
	}
	if err := m.store.AckMessage(ctx, messageID, result); err != nil {
		return err
	}
	msg, err := m.store.GetMessage(ctx, messageID)
	if err == nil {
		m.notifyWake(msg.ThreadID, msg.RunID)
	}
	return nil
}

func (m *Manager) NackMessage(ctx context.Context, messageID string, reason string, retryAt *time.Time) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("message store is not configured")
	}
	if err := m.store.NackMessage(ctx, messageID, reason, retryAt); err != nil {
		return err
	}
	msg, err := m.store.GetMessage(ctx, messageID)
	if err == nil {
		m.notifyWake(msg.ThreadID, msg.RunID)
	}
	return nil
}

func (m *Manager) RequeueExpiredClaims(ctx context.Context) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("message store is not configured")
	}
	return m.store.RequeueExpiredClaims(ctx)
}

func (m *Manager) GetMessage(ctx context.Context, messageID string) (types.AgentMessage, error) {
	if m == nil || m.store == nil {
		return types.AgentMessage{}, fmt.Errorf("message store is not configured")
	}
	return m.store.GetMessage(ctx, messageID)
}

func (m *Manager) ListMessages(ctx context.Context, filter state.MessageFilter) ([]types.AgentMessage, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("message store is not configured")
	}
	return m.store.ListMessages(ctx, filter)
}

func (m *Manager) CountMessages(ctx context.Context, filter state.MessageFilter) (int, error) {
	if m == nil || m.store == nil {
		return 0, fmt.Errorf("message store is not configured")
	}
	return m.store.CountMessages(ctx, filter)
}
