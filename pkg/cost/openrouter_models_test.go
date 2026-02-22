package cost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestContextLengthFromOpenRouter_FailBackoff(t *testing.T) {
	ctx := context.Background()

	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Reset cache globals
	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	orCacheModTime = time.Time{}
	orCacheFailTime = time.Time{}
	origBackoff := orFailBackoff
	orFailBackoff = 1 * time.Hour // large backoff so retries stay suppressed
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
		orCacheModTime = time.Time{}
		orCacheFailTime = time.Time{}
		orFailBackoff = origBackoff
	})

	// First call triggers a fetch attempt that fails.
	_, ok := ContextLengthFromOpenRouter(ctx, "some-model")
	if ok {
		t.Fatal("expected false for failed fetch")
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected exactly 1 fetch, got %d", fetchCount.Load())
	}

	// Subsequent calls within the backoff window must NOT hit the server again.
	for i := 0; i < 5; i++ {
		_, _ = ContextLengthFromOpenRouter(ctx, "some-model")
	}
	if fetchCount.Load() != 1 {
		t.Fatalf("expected fetch count to remain 1 during backoff, got %d", fetchCount.Load())
	}

	// After the backoff expires, a retry should occur.
	orCacheMu.Lock()
	orCacheFailTime = time.Now().Add(-2 * time.Hour)
	orCacheMu.Unlock()

	_, _ = ContextLengthFromOpenRouter(ctx, "some-model")
	if fetchCount.Load() != 2 {
		t.Fatalf("expected fetch count to be 2 after backoff expiry, got %d", fetchCount.Load())
	}
}
