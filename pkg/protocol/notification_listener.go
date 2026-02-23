package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// RunNotificationListener connects to the RPC endpoint and reads notifications indefinitely.
// It calls onNotification for every pushed notification message.
// It returns an error if the connection fails or if the context is canceled.
func RunNotificationListener(ctx context.Context, endpoint string, onNotification func(msg Message) error) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = DefaultRPCEndpoint
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return fmt.Errorf("connect notification endpoint %s: %w", endpoint, err)
	}
	defer conn.Close()

	// Send a dummy request to establish intention, or just read?
	// The daemon broadcasts immediately to all connected sockets.
	// But actually, we don't need to send anything. We just read.

	dec := json.NewDecoder(conn)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Don't set deadline on read, as notifications can be sparse.
		// Alternatively, set a long deadline and continue on timeout.
		var msg Message
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return fmt.Errorf("notification connection closed")
			}
			return fmt.Errorf("decode notification message: %w", err)
		}

		// Notifications have ID == nil
		if msg.ID == nil {
			if err := onNotification(msg); err != nil {
				return err
			}
		}
	}
}
