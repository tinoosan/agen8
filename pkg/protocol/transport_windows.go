//go:build windows

package protocol

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialNamedPipe(ctx context.Context, target string, timeout time.Duration) (net.Conn, error) {
	path := namedPipePath(target)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return winio.DialPipeContext(ctx, path)
}

func listenNamedPipe(target string) (net.Listener, error) {
	path := namedPipePath(target)
	return winio.ListenPipe(path, nil)
}

func namedPipePath(target string) string {
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(target, `\\.\pipe\`)
	target = strings.TrimPrefix(target, "/")
	if target == "" {
		target = "agen8-daemon"
	}
	return fmt.Sprintf(`\\.\pipe\%s`, target)
}
