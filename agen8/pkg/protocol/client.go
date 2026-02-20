package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

const DefaultRPCEndpoint = "127.0.0.1:7777"

var rpcRequestSeq uint64

// TCPClient is a minimal JSON-RPC client over TCP.
type TCPClient struct {
	Endpoint string
	Timeout  time.Duration
}

func (c TCPClient) Call(ctx context.Context, method string, params any, out any) error {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultRPCEndpoint
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return fmt.Errorf("connect rpc endpoint %s: %w", endpoint, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	reqID := fmt.Sprintf("%d", atomic.AddUint64(&rpcRequestSeq, 1))
	req, err := NewRequest(reqID, method, params)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("encode rpc request: %w", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}

	dec := json.NewDecoder(conn)
	for {
		var msg Message
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return fmt.Errorf("rpc %s: no response", method)
			}
			return fmt.Errorf("decode rpc response: %w", err)
		}
		if msg.ID == nil || strings.TrimSpace(*msg.ID) != reqID {
			// Ignore notifications/other messages and keep scanning for our response.
			continue
		}
		if msg.Error != nil {
			return &ProtocolError{Code: msg.Error.Code, Message: strings.TrimSpace(msg.Error.Message)}
		}
		if out == nil {
			return nil
		}
		if len(msg.Result) == 0 {
			return nil
		}
		return json.Unmarshal(msg.Result, out)
	}
}
