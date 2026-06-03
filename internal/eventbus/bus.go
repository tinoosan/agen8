package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/google/uuid"
)

// Bus wraps a Watermill GoChannel publisher/subscriber for in-process
// domain event delivery. It is the single entry point for publishing
// and subscribing to domain events within the daemon.
type Bus struct {
	pubSub *gochannel.GoChannel
	router *message.Router
	logger watermill.LoggerAdapter

	mu       sync.Mutex
	handlers []handlerRegistration
	running  bool
	ready    chan struct{}
}

type handlerRegistration struct {
	name           string
	subscribeTopic string
	handlerFunc    message.NoPublishHandlerFunc
}

// New creates a new event bus using Watermill's in-process GoChannel backend.
func New(logger watermill.LoggerAdapter) *Bus {
	if logger == nil {
		logger = watermill.NewSlogLogger(slog.Default())
	}
	pubSub := gochannel.NewGoChannel(gochannel.Config{
		OutputChannelBuffer: 256,
		Persistent:          false,
	}, logger)

	return &Bus{
		pubSub: pubSub,
		logger: logger,
		ready:  make(chan struct{}),
	}
}

// Publish serializes an event and publishes it to the given topic.
func (b *Bus) Publish(topic string, event any) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("topic is required")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	msg := message.NewMessage(uuid.NewString(), payload)
	return b.pubSub.Publish(topic, msg)
}

// AddHandler registers a handler that subscribes to a topic. Handlers must be
// added before Run is called. The handler receives raw Watermill messages;
// callers are responsible for deserializing the payload.
func (b *Bus) AddHandler(name, subscribeTopic string, handlerFunc message.NoPublishHandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, handlerRegistration{
		name:           name,
		subscribeTopic: subscribeTopic,
		handlerFunc:    handlerFunc,
	})
}

// Run starts the Watermill router. It blocks until ctx is cancelled.
func (b *Bus) Run(ctx context.Context) error {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return fmt.Errorf("bus is already running")
	}
	b.running = true

	router, err := message.NewRouter(message.RouterConfig{}, b.logger)
	if err != nil {
		b.running = false
		b.mu.Unlock()
		return fmt.Errorf("create router: %w", err)
	}
	b.router = router

	for _, h := range b.handlers {
		router.AddConsumerHandler(h.name, h.subscribeTopic, b.pubSub, h.handlerFunc)
	}
	b.mu.Unlock()

	go func() {
		<-router.Running()
		close(b.ready)
	}()

	return router.Run(ctx)
}

// Running is closed after the underlying router has subscribed all registered
// handlers and can receive domain events.
func (b *Bus) Running() <-chan struct{} {
	if b == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return b.ready
}

// Close gracefully shuts down the bus.
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.router != nil {
		return b.router.Close()
	}
	return b.pubSub.Close()
}

// Subscribe returns a raw message channel for a topic. This is a low-level
// API for cases where a handler registration is not suitable (e.g., feeding
// a channel to a select loop). The caller must call the returned cancel func
// on shutdown.
func (b *Bus) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	return b.pubSub.Subscribe(ctx, topic)
}
