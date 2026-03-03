package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/tinoosan/agen8/pkg/protocol"
)

func serveRPCOverTCP(ctx context.Context, addr string, srv *RPCServer) error {
	if srv == nil {
		return fmt.Errorf("rpc server is nil")
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen rpc tcp %s: %w", addr, err)
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				log.Printf("daemon: rpc accept error: %v", err)
				continue
			}
			go func(c net.Conn) {
				cancelWatchDone := make(chan struct{})
				go func() {
					select {
					case <-ctx.Done():
						_ = c.Close()
					case <-cancelWatchDone:
					}
				}()
				defer close(cancelWatchDone)
				defer c.Close()
				if err := srv.Serve(ctx, c, c); err != nil && ctx.Err() == nil {
					log.Printf("daemon: rpc connection closed: %v", err)
				}
			}(conn)
		}
	}()
	return nil
}

// serveRPCOverTCPWithBroadcaster listens on addr and serves each connection with a dedicated
// RPCServer whose NotifyCh is a subscriber of the broadcaster. When a client connects, a new
// channel is registered, configForNotifyCh is called to build RPCServerConfig with that channel,
// and the server runs until the connection closes, then the channel is unregistered.
func serveRPCOverTCPWithBroadcaster(ctx context.Context, addr string, broadcaster *EventBroadcaster, configForNotifyCh func(notifyCh <-chan protocol.Message) RPCServerConfig) error {
	if broadcaster == nil || configForNotifyCh == nil {
		return fmt.Errorf("broadcaster and configForNotifyCh are required")
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen rpc tcp %s: %w", addr, err)
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return
				}
				log.Printf("daemon: rpc accept error: %v", err)
				continue
			}
			go func(c net.Conn) {
				cancelWatchDone := make(chan struct{})
				go func() {
					select {
					case <-ctx.Done():
						_ = c.Close()
					case <-cancelWatchDone:
					}
				}()
				defer close(cancelWatchDone)
				defer c.Close()
				notifyCh := make(chan protocol.Message, 64)
				broadcaster.Register(notifyCh)
				defer broadcaster.Unregister(notifyCh)
				cfg := configForNotifyCh(notifyCh)
				srv := NewRPCServer(cfg)
				if err := srv.Serve(ctx, c, c); err != nil && ctx.Err() == nil {
					log.Printf("daemon: rpc connection closed: %v", err)
				}
			}(conn)
		}
	}()
	return nil
}
