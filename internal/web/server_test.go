package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAFileServer_ServesExistingAssets(t *testing.T) {
	t.Parallel()

	handler := spaFileServer(fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte("<html>index</html>")},
		"assets/app.js":  &fstest.MapFile{Data: []byte("console.log('app')")},
		"assets/app.css": &fstest.MapFile{Data: []byte("body{}")},
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "console.log('app')" {
		t.Fatalf("body=%q want asset body", got)
	}
}

func TestSPAFileServer_FallsBackToIndexForClientRoutes(t *testing.T) {
	t.Parallel()

	handler := spaFileServer(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>index shell</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/spaces/alpha/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "index shell") {
		t.Fatalf("body=%q want SPA index", rec.Body.String())
	}
}

func TestSPAFileServer_FallsBackWhenAssetLookupErrors(t *testing.T) {
	t.Parallel()

	handler := spaFileServer(failingFS{})

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusNotFound)
	}
}

type failingFS struct{}

func (failingFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
