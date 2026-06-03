package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

var rpcRequestSeq uint64

// RPCClient is a minimal JSON-RPC client over the configured local transport.
type RPCClient struct {
	Endpoint string
	Timeout  time.Duration
}

// TCPClient is kept as a back-compat alias for existing call sites.
type TCPClient = RPCClient

func DefaultRPCEndpoint() string {
	return defaultLocalEndpoint()
}

func (c RPCClient) Call(ctx context.Context, method string, params any, out any) error {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = DefaultRPCEndpoint()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	conn, err := DialContext(ctx, endpoint, timeout)
	if err != nil {
		return fmt.Errorf("connect rpc endpoint %s: %w", endpoint, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set rpc deadline: %w", err)
	}

	reqID := fmt.Sprintf("%d", atomic.AddUint64(&rpcRequestSeq, 1))
	req, err := NewRequest(reqID, method, params)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("encode rpc request: %w", err)
	}
	closeWrite(conn)

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
			continue
		}
		if msg.Error != nil {
			return &ProtocolError{Code: msg.Error.Code, Message: strings.TrimSpace(msg.Error.Message)}
		}
		if out == nil || len(msg.Result) == 0 {
			return nil
		}
		return json.Unmarshal(msg.Result, out)
	}
}
