package app

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/emit"
	"github.com/tinoosan/agen8/pkg/protocol"
	eventsvc "github.com/tinoosan/agen8/pkg/services/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// ProtocolInitializer encapsulates protocol index warmup, notification fanout,
// and server startup wiring for daemon mode.
type ProtocolInitializer struct {
	cfg     config.Config
	run     types.Run
	enabled bool

	eventsLister *eventsvc.Service // optional; used for warmup when run has RunID
	index        *protocol.Index
	notifyCh     chan protocol.Message
}

func newProtocolInitializer(cfg config.Config, run types.Run, enabled bool, eventsLister *eventsvc.Service) *ProtocolInitializer {
	return &ProtocolInitializer{cfg: cfg, run: run, enabled: enabled, eventsLister: eventsLister}
}

func shouldEnableProtocolStdio(explicit bool, inTTY, outTTY bool) bool {
	if explicit {
		return true
	}
	return !inTTY && !outTTY
}

func (p *ProtocolInitializer) Enabled() bool {
	if p == nil {
		return false
	}
	return p.enabled
}

func (p *ProtocolInitializer) Index() *protocol.Index {
	if p == nil {
		return nil
	}
	return p.index
}

func (p *ProtocolInitializer) NotifyCh() chan protocol.Message {
	if p == nil {
		return nil
	}
	return p.notifyCh
}

func (p *ProtocolInitializer) Initialize(ctx context.Context) {
	if p == nil || !p.enabled {
		return
	}
	p.index = protocol.NewIndex(10_000, 2_000)
	p.notifyCh = make(chan protocol.Message, 1000)

	// No run to warm up when daemon has no bootstrap run (clean server model).
	if strings.TrimSpace(p.run.RunID) == "" {
		return
	}

	replaySink := protocol.NewEventSink(
		emit.SinkFunc[protocol.Notification](func(_ context.Context, msg emit.Message[protocol.Notification]) error {
			p.index.Apply(msg.Payload.Method, msg.Payload.Params)
			return nil
		}),
		protocol.WithThreadID(protocol.ThreadID(p.run.SessionID)),
	)

	if p.eventsLister == nil {
		return
	}
	var after int64
	for {
		batch, next, err := p.eventsLister.ListPaginated(ctx, eventsvc.Filter{
			RunID:    p.run.RunID,
			Limit:    1000,
			AfterSeq: after,
		})
		if err != nil {
			log.Printf("daemon: protocol warmup failed: %v", err)
			break
		}
		if len(batch) == 0 {
			break
		}
		for _, ev := range batch {
			_ = replaySink.Emit(ctx, emit.Message[types.EventRecord]{RunID: p.run.RunID, Payload: ev})
		}
		after = next
	}
}

func (p *ProtocolInitializer) NewProtocolSink() *protocol.EventSink {
	if p == nil {
		return nil
	}
	return protocol.NewEventSink(
		emit.SinkFunc[protocol.Notification](func(_ context.Context, msg emit.Message[protocol.Notification]) error {
			if p.index != nil {
				p.index.Apply(msg.Payload.Method, msg.Payload.Params)
			}
			if p.notifyCh == nil {
				return nil
			}
			out, err := protocol.NewNotification(msg.Payload.Method, msg.Payload.Params)
			if err != nil {
				return err
			}
			select {
			case p.notifyCh <- out:
			default:
				// Drop if buffer is full (best-effort).
			}
			return nil
		}),
		protocol.WithThreadID(protocol.ThreadID(p.run.SessionID)),
	)
}

func (p *ProtocolInitializer) StartServers(ctx context.Context, srvCfg RPCServerConfig, listenAddr string) error {
	if p == nil || !p.enabled {
		return nil
	}
	srv := NewRPCServer(srvCfg)
	go func() {
		if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
			log.Printf("daemon: protocol server stopped: %v", err)
		}
	}()

	tcpCfg := srvCfg
	tcpCfg.NotifyCh = nil
	tcpCfg.Index = nil
	tcpSrv := NewRPCServer(tcpCfg)
	if err := serveRPCOverTCP(ctx, strings.TrimSpace(listenAddr), tcpSrv); err != nil {
		return err
	}
	return nil
}
