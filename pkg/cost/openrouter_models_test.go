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
		w.Write([]byte(`{"data":[{"id":"openrouter/mock-model","context_length":99000,"supported_parameters":["reasoning","temperature"]}]}`))
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

func TestSupportsReasoningSummaryFromOpenRouter(t *testing.T) {
	ctx := context.Background()

	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"id":"openrouter/reasoning-model","context_length":128000,"supported_parameters":["reasoning","temperature"]},
			{"id":"openrouter/nonreasoning-model","context_length":128000,"supported_parameters":["temperature"]}
		]}`))
	}))
	defer srv.Close()

	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	orCacheModTime = time.Time{}
	orCacheFailTime = time.Time{}
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
		orCacheModTime = time.Time{}
		orCacheFailTime = time.Time{}
	})

	if supports, known := SupportsReasoningSummaryFromOpenRouter(ctx, "openrouter/reasoning-model"); !known || !supports {
		t.Fatalf("expected reasoning support=true, known=true; got supports=%v known=%v", supports, known)
	}
	if supports, known := SupportsReasoningSummaryFromOpenRouter(ctx, "openrouter/nonreasoning-model"); !known || supports {
		t.Fatalf("expected reasoning support=false, known=true; got supports=%v known=%v", supports, known)
	}
	if supports, known := SupportsReasoningSummaryFromOpenRouter(ctx, "openrouter/unknown-model"); known || supports {
		t.Fatalf("expected unknown model to return supports=false, known=false; got supports=%v known=%v", supports, known)
	}
}

func TestOpenRouterModelInfos_IncludesPricingPerM(t *testing.T) {
	ctx := context.Background()

	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{
				"id":"moonshotai/kimi-k2.5",
				"supported_parameters":["reasoning","temperature"],
				"pricing":{"prompt":"0.00000045","completion":"0.0000022"},
				"top_provider":{"context_length":262144}
			}
		]}`))
	}))
	defer srv.Close()

	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	orCacheModTime = time.Time{}
	orCacheFailTime = time.Time{}
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
		orCacheModTime = time.Time{}
		orCacheFailTime = time.Time{}
	})

	infos, ok := OpenRouterModelInfos(ctx)
	if !ok || len(infos) != 1 {
		t.Fatalf("expected one model info; ok=%v len=%d", ok, len(infos))
	}
	info := infos[0]
	if info.ID != "moonshotai/kimi-k2.5" {
		t.Fatalf("id=%q", info.ID)
	}
	if diff := info.InputPerM - 0.45; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("inputPerM=%v, want 0.45", info.InputPerM)
	}
	if diff := info.OutputPerM - 2.2; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("outputPerM=%v, want 2.2", info.OutputPerM)
	}
	if info.ContextLength != 262144 {
		t.Fatalf("context=%d, want 262144", info.ContextLength)
	}
	if !info.IsReasoning {
		t.Fatalf("expected IsReasoning=true")
	}
}

func TestSupportsReasoningSummaryFromOpenRouterCached_DoesNotFetch(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"openrouter/reasoning-model","supported_parameters":["reasoning"]}]}`))
	}))
	defer srv.Close()

	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	orCacheModTime = time.Time{}
	orCacheFailTime = time.Time{}
	orRefreshMu.Lock()
	orRefreshActive = false
	orRefreshMu.Unlock()
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
		orCacheModTime = time.Time{}
		orCacheFailTime = time.Time{}
		orRefreshMu.Lock()
		orRefreshActive = false
		orRefreshMu.Unlock()
	})

	if supports, known := SupportsReasoningSummaryFromOpenRouterCached("openrouter/reasoning-model"); known || supports {
		t.Fatalf("expected cache miss without network fetch; got supports=%v known=%v", supports, known)
	}
	if got := fetchCount.Load(); got != 0 {
		t.Fatalf("expected no network fetch for cached lookup, got %d", got)
	}
}

func TestTriggerOpenRouterModelRefreshAsync_WarmsCache(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Cleanup(func() { os.Unsetenv("OPENROUTER_API_KEY") })

	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"openrouter/reasoning-model","supported_parameters":["reasoning","temperature"]}]}`))
	}))
	defer srv.Close()

	openRouterAPIURL = srv.URL
	orCacheMu = sync.RWMutex{}
	orModelCache = nil
	orCacheModTime = time.Time{}
	orCacheFailTime = time.Time{}
	orRefreshMu.Lock()
	orRefreshActive = false
	orRefreshMu.Unlock()
	t.Cleanup(func() {
		openRouterAPIURL = "https://openrouter.ai/api/v1/models"
		orModelCache = nil
		orCacheModTime = time.Time{}
		orCacheFailTime = time.Time{}
		orRefreshMu.Lock()
		orRefreshActive = false
		orRefreshMu.Unlock()
	})

	TriggerOpenRouterModelRefreshAsync(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if supports, known := SupportsReasoningSummaryFromOpenRouterCached("openrouter/reasoning-model"); known && supports {
			if fetchCount.Load() == 0 {
				t.Fatalf("expected at least one refresh fetch")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for async refresh to populate cache; fetches=%d", fetchCount.Load())
}
