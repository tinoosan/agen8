// Package web provides the HTTP server for the agen8 web UI.
// It serves embedded static frontend assets and bridges browser JSON-RPC
// calls to the daemon TCP socket.
package web

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

//go:embed all:dist
var staticFiles embed.FS

// Server serves the agen8 web UI and bridges RPC to the daemon.
type Server struct {
	// Addr is the HTTP listen address, e.g. ":8080".
	Addr string
	// RPCEndpoint is the daemon JSON-RPC TCP address, e.g. "127.0.0.1:7777".
	RPCEndpoint string
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	addr := strings.TrimSpace(s.Addr)
	if addr == "" {
		addr = ":8080"
	}
	rpc := strings.TrimSpace(s.RPCEndpoint)
	if rpc == "" {
		rpc = "127.0.0.1:7777"
	}

	mux := http.NewServeMux()

	// CORS preflight.
	mux.HandleFunc("OPTIONS /rpc", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	// JSON-RPC proxy: forward individual requests to the daemon.
	mux.HandleFunc("POST /rpc", func(w http.ResponseWriter, r *http.Request) {
		handleRPC(w, r, rpc)
	})

	// Server-Sent Events: stream daemon notifications to the browser.
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		handleEvents(w, r, rpc)
	})

	// Static files: serve the compiled frontend SPA.
	staticFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return err
	}
	mux.Handle("/", spaFileServer(staticFS))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info("web UI started", "component", "web", "addr", ln.Addr().String(), "rpc_endpoint", rpc)

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// spaFileServer returns a handler that serves files from fsys and falls back
// to index.html for any path that doesn't resolve to a real file (SPA routing).
func spaFileServer(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try the requested path; if it opens cleanly it's a real asset.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := fsys.Open(path)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Not a real file — serve index.html for client-side routing.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
