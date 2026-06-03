//go:build !windows

package protocol

import (
	"context"
	"fmt"
	"net"
	"time"
)

func dialNamedPipe(_ context.Context, target string, _ time.Duration) (net.Conn, error) {
	return nil, fmt.Errorf("named pipes are only supported on windows (%s)", target)
}

func listenNamedPipe(target string) (net.Listener, error) {
	return nil, fmt.Errorf("named pipes are only supported on windows (%s)", target)
}
