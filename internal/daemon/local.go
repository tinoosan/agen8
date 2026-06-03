package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	osuser "os/user"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/rpc"
	userdomain "github.com/tinoosan/agen8-mcp-server/internal/services/user/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

type LocalStrategy struct{}

func (LocalStrategy) Run(ctx context.Context, d *Daemon) error {
	ln, endpoint, err := protocol.Listen(d.cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("listen local rpc: %w", err)
	}
	defer ln.Close()

	httpLn, err := net.Listen("tcp", d.cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen local web ui: %w", err)
	}
	defer httpLn.Close()

	errCh := make(chan error, 1)
	go func() {
		if err := d.serveHTTP(ctx, httpLn); err != nil {
			errCh <- err
			_ = ln.Close()
		}
	}()

	if d.cfg.Out != nil {
		fmt.Fprintf(d.cfg.Out, "agen8 daemon listening on %s\n", endpoint)
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		select {
		case err := <-errCh:
			return err
		default:
		}
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case httpErr := <-errCh:
				return httpErr
			default:
			}
			return fmt.Errorf("accept local rpc connection: %w", err)
		}
		if err := d.serveLocalConnection(ctx, conn); err != nil {
			conn.Close()
			return err
		}
	}
}

func (d *Daemon) serveLocalConnection(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	body, err := io.ReadAll(conn)
	if err != nil {
		return fmt.Errorf("read local rpc request: %w", err)
	}
	identity, err := localIdentity()
	if err != nil {
		return err
	}
	ctx = rpc.ContextWithIdentity(ctx, identity)
	resp, err := d.rpc.Handle(ctx, body)
	if err != nil {
		return fmt.Errorf("handle local rpc request: %w", err)
	}
	if len(resp) == 0 {
		resp, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": -32603, "message": "empty rpc response"}})
	}
	if _, err := conn.Write(append(resp, '\n')); err != nil {
		return fmt.Errorf("write local rpc response: %w", err)
	}
	return nil
}

func localIdentity() (rpc.Identity, error) {
	current, err := osuser.Current()
	if err != nil {
		return rpc.Identity{}, fmt.Errorf("resolve local user: %w", err)
	}
	id := strings.TrimSpace(current.Uid)
	if id == "" {
		id = strings.TrimSpace(current.Username)
	}
	if id == "" {
		return rpc.Identity{}, fmt.Errorf("local user id is empty")
	}
	return rpc.Identity{UserID: id, Role: string(userdomain.RoleAdmin)}, nil
}
