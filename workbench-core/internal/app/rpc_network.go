package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
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
				defer c.Close()
				if err := srv.Serve(ctx, c, c); err != nil && ctx.Err() == nil {
					log.Printf("daemon: rpc connection closed: %v", err)
				}
			}(conn)
		}
	}()
	return nil
}
