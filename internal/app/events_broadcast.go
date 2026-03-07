package app

import (
	"context"
	"sync"

	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/protocol"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// EventBroadcaster fans out event.append notifications to multiple subscriber channels.
// Run(ctx) must be started; Register/Unregister are safe for concurrent use.
type EventBroadcaster struct {
	mu          sync.RWMutex
	subs        map[chan protocol.Message]struct{}
	broadcastCh chan protocol.Message
}

// NewEventBroadcaster returns a broadcaster and the write-only channel that appenders use.
// The caller must call Run(ctx) in a goroutine. Sends to the channel are non-blocking
// (drops if full) so appenders never block.
func NewEventBroadcaster() (*EventBroadcaster, chan<- protocol.Message) {
	ch := make(chan protocol.Message, 256)
	return &EventBroadcaster{
		subs:        make(map[chan protocol.Message]struct{}),
		broadcastCh: ch,
	}, ch
}

// Register adds a subscriber channel. The channel will receive event.append notifications.
func (b *EventBroadcaster) Register(ch chan protocol.Message) {
	if ch == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
}

// Unregister removes the subscriber and closes the channel.
func (b *EventBroadcaster) Unregister(ch chan protocol.Message) {
	if ch == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
	close(ch)
}

// Run reads from the broadcast channel and sends each message to all registered subscribers.
// Non-blocking send per subscriber so one slow client does not block others.
// Exits when ctx is done.
func (b *EventBroadcaster) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-b.broadcastCh:
			if !ok {
				return
			}
			b.mu.RLock()
			for ch := range b.subs {
				select {
				case ch <- msg:
				default:
					// Best-effort: drop if subscriber is slow
				}
			}
			b.mu.RUnlock()
		}
	}
}

func (b *EventBroadcaster) Broadcast(msg protocol.Message) {
	if b == nil || b.broadcastCh == nil {
		return
	}
	select {
	case b.broadcastCh <- msg:
	default:
	}
}

func (b *EventBroadcaster) Notify(method string, params any) error {
	if b == nil {
		return nil
	}
	msg, err := protocol.NewNotification(method, params)
	if err != nil {
		return err
	}
	b.Broadcast(msg)
	return nil
}

// broadcastingEventsAppender wraps an EventsAppender and broadcasts every Append/AppendEvent
// as a protocol notification to the given channel. Implements both app.EventsAppender
// and pkg/events.StoreAppender so it can be used as EventsService and EventsStore.
type broadcastingEventsAppender struct {
	inner       EventsAppender
	broadcastCh chan<- protocol.Message
}

// NewBroadcastingEventsAppender returns an EventsAppender that delegates to inner and
// sends event.append notifications to broadcastCh. Also implements pkg/events.StoreAppender.
func NewBroadcastingEventsAppender(inner EventsAppender, broadcastCh chan<- protocol.Message) *broadcastingEventsAppender {
	return &broadcastingEventsAppender{inner: inner, broadcastCh: broadcastCh}
}

// Append implements app.EventsAppender.
func (b *broadcastingEventsAppender) Append(ctx context.Context, event types.EventRecord) error {
	err := b.inner.Append(ctx, event)
	if err == nil {
		b.sendNotification(event)
	}
	return err
}

// AppendEvent implements pkg/events.StoreAppender (for supervisor StoreSink).
func (b *broadcastingEventsAppender) AppendEvent(ctx context.Context, event types.EventRecord) error {
	err := b.inner.Append(ctx, event)
	if err == nil {
		b.sendNotification(event)
	}
	return err
}

// ListPaginated implements app.EventsAppender.
func (b *broadcastingEventsAppender) ListPaginated(ctx context.Context, filter eventsvc.Filter) ([]types.EventRecord, int64, error) {
	return b.inner.ListPaginated(ctx, filter)
}

// LatestSeq implements app.EventsAppender.
func (b *broadcastingEventsAppender) LatestSeq(ctx context.Context, runID string) (int64, error) {
	return b.inner.LatestSeq(ctx, runID)
}

// Count implements app.EventsAppender.
func (b *broadcastingEventsAppender) Count(ctx context.Context, filter eventsvc.Filter) (int, error) {
	return b.inner.Count(ctx, filter)
}

func (b *broadcastingEventsAppender) sendNotification(event types.EventRecord) {
	if b.broadcastCh == nil {
		return
	}
	msg, err := protocol.NewNotification(protocol.NotifyEventAppend, event)
	if err != nil {
		return
	}
	select {
	case b.broadcastCh <- msg:
	default:
	}
}

var (
	_ EventsAppender       = (*broadcastingEventsAppender)(nil)
	_ events.StoreAppender = (*broadcastingEventsAppender)(nil)
)
