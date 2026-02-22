package cost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestContextLengthFromOpenRouter(t *testing.T) {
	ctx := context.Background()

	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"openrouter/mock-model","context_length":99000}]}`))
	}))
	defer srv.Close()

	// Override API URL and cache globals
	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
	})

	// 1. Valid lookup
	l, ok := ContextLengthFromOpenRouter(ctx, "openrouter/mock-model")
	if !ok || l != 99000 {
		t.Fatalf("expected 99000, true - got %d, %v", l, ok)
	}

	// 2. Cache hit (server closed)
	srv.Close()
	l, ok = ContextLengthFromOpenRouter(ctx, "openrouter/mock-model")
	if !ok || l != 99000 {
		t.Fatalf("expected 99000 from cache, true - got %d, %v", l, ok)
	}

	// 3. Fallback suffix lookup on cache hit
	l, ok = ContextLengthFromOpenRouter(ctx, "mock-model")
	if !ok || l != 99000 {
		t.Fatalf("expected 99000 from suffix match, true - got %d, %v", l, ok)
	}

	// 4. Unknown model (should return 0, false)
	l, ok = ContextLengthFromOpenRouter(ctx, "unknown/model")
	if ok || l != 0 {
		t.Fatalf("expected 0, false - got %d, %v", l, ok)
	}

	// 5. Without API key
	os.Setenv("OPENROUTER_API_KEY", "")
	l, ok = ContextLengthFromOpenRouter(ctx, "openrouter/mock-model")
	if ok || l != 0 {
		t.Fatalf("expected 0, false without API key")
	}
}
