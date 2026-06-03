package protocol

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Endpoint struct {
	Scheme string
	Target string
}

func ParseEndpoint(raw string) Endpoint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultRPCEndpoint()
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil {
			switch parsed.Scheme {
			case "tcp":
				return Endpoint{Scheme: "tcp", Target: parsed.Host}
			case "unix":
				return Endpoint{Scheme: "unix", Target: parsed.Path}
			case "npipe":
				target := parsed.Host + parsed.Path
				target = strings.TrimPrefix(target, "/")
				return Endpoint{Scheme: "npipe", Target: target}
			}
		}
	}
	if strings.HasPrefix(raw, "/") {
		return Endpoint{Scheme: "unix", Target: raw}
	}
	return Endpoint{Scheme: "tcp", Target: raw}
}

func DialContext(ctx context.Context, endpoint string, timeout time.Duration) (net.Conn, error) {
	ep := ParseEndpoint(endpoint)
	switch ep.Scheme {
	case "unix":
		var d net.Dialer
		if timeout > 0 {
			d.Timeout = timeout
		}
		return d.DialContext(ctx, "unix", ep.Target)
	case "npipe":
		return dialNamedPipe(ctx, ep.Target, timeout)
	default:
		var d net.Dialer
		if timeout > 0 {
			d.Timeout = timeout
		}
		return d.DialContext(ctx, "tcp", ep.Target)
	}
}

func Listen(endpoint string) (net.Listener, string, error) {
	ep := ParseEndpoint(endpoint)
	switch ep.Scheme {
	case "unix":
		return listenUnix(ep.Target)
	case "npipe":
		ln, err := listenNamedPipe(ep.Target)
		return ln, endpointString(ep), err
	default:
		ln, err := net.Listen("tcp", ep.Target)
		if err != nil {
			return nil, "", err
		}
		return ln, endpointString(Endpoint{Scheme: "tcp", Target: ln.Addr().String()}), nil
	}
}

func endpointString(ep Endpoint) string {
	switch ep.Scheme {
	case "unix":
		return "unix://" + ep.Target
	case "npipe":
		return "npipe://" + ep.Target
	default:
		return ep.Target
	}
}

func closeWrite(conn net.Conn) error {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return nil
}

func defaultLocalEndpoint() string {
	if runtime.GOOS == "windows" {
		return "npipe://agen8-daemon"
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "unix://" + filepath.Join(os.TempDir(), "agen8-daemon.sock")
	}
	return "unix://" + filepath.Join(home, ".agen8", "daemon.sock")
}

func listenUnix(path string) (net.Listener, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", fmt.Errorf("unix endpoint path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", err
	}
	return &cleanupListener{Listener: ln, cleanupPath: path}, endpointString(Endpoint{Scheme: "unix", Target: path}), nil
}

type cleanupListener struct {
	net.Listener
	cleanupPath string
}

func (l *cleanupListener) Close() error {
	err := l.Listener.Close()
	if strings.TrimSpace(l.cleanupPath) != "" {
		os.Remove(l.cleanupPath)
	}
	return err
}
