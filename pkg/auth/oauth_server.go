package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
)

const (
	OAuthCallbackAddr = "127.0.0.1:1455"
	OAuthCallbackPath = "/auth/callback"
)

type OAuthCallbackServer struct {
	ready bool
	close func()
	wait  func(ctx context.Context) (string, error)
}

func (s *OAuthCallbackServer) Ready() bool {
	if s == nil {
		return false
	}
	return s.ready
}

func (s *OAuthCallbackServer) Close() {
	if s == nil || s.close == nil {
		return
	}
	s.close()
}

func (s *OAuthCallbackServer) WaitForCode(ctx context.Context) (string, error) {
	if s == nil || s.wait == nil {
		return "", fmt.Errorf("oauth callback server is unavailable")
	}
	return s.wait(ctx)
}

func StartOAuthCallbackServer(state string) (*OAuthCallbackServer, error) {
	state = strings.TrimSpace(state)
	if state == "" {
		return nil, fmt.Errorf("oauth state is required")
	}
	ln, err := net.Listen("tcp", OAuthCallbackAddr)
	if err != nil {
		// Caller will fall back to manual code copy/paste mode.
		return &OAuthCallbackServer{ready: false, close: func() {}, wait: func(context.Context) (string, error) {
			return "", fmt.Errorf("oauth callback server unavailable: %w", err)
		}}, nil
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc(OAuthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if strings.TrimSpace(q.Get("state")) != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("oauth state mismatch"):
			default:
			}
			return
		}
		code := strings.TrimSpace(q.Get("code"))
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("oauth callback missing code"):
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><h3>Login complete. You can close this tab.</h3></body></html>"))
		select {
		case codeCh <- code:
		default:
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			select {
			case errCh <- serveErr:
			default:
			}
		}
	}()

	closeFn := func() {
		once.Do(func() {
			_ = srv.Shutdown(context.Background())
		})
	}
	waitFn := func(ctx context.Context) (string, error) {
		defer closeFn()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case code := <-codeCh:
			return code, nil
		case werr := <-errCh:
			return "", werr
		}
	}

	return &OAuthCallbackServer{ready: true, close: closeFn, wait: waitFn}, nil
}
