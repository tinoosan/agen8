// Package web provides the embedded agen8 browser UI.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// DefaultWebPort is the default HTTP server port for the web UI.
const DefaultWebPort = 8080

// all:dist embeds the placeholder .gitkeep too, so the package compiles even
// when the generated bundle is absent (the bundle is no longer committed; it is
// built by `make build`/`npm run build`). See internal/web/dist/.gitkeep.
//
//go:embed all:dist
var staticFiles embed.FS

// Handler returns an HTTP handler for the compiled frontend SPA.
func Handler() (http.Handler, error) {
	staticFS, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		return nil, err
	}
	return spaFileServer(staticFS), nil
}

// Mount attaches the embedded SPA to mux. Protocol routes must be registered
// before this because "/" intentionally catches every remaining path.
func Mount(mux *http.ServeMux) error {
	handler, err := Handler()
	if err != nil {
		return err
	}
	mux.Handle("/", handler)
	return nil
}

// spaFileServer returns a handler that serves files from fsys and falls back
// to index.html for any path that doesn't resolve to a real file.
func spaFileServer(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := fsys.Open(path)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
