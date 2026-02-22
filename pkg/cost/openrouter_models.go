package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type openRouterModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		ContextLength int    `json:"context_length"`
	} `json:"data"`
}

var (
	orModelCache    map[string]int
	orCacheModTime  time.Time
	orCacheFailTime time.Time
	orCacheMu       sync.RWMutex
	orCacheTTL      = 1 * time.Hour
	orFailBackoff   = 2 * time.Minute
)

// ContextLengthFromOpenRouter fetches the context length for a given model from OpenRouter.
func ContextLengthFromOpenRouter(ctx context.Context, modelID string) (int, bool) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		return 0, false
	}

	id := normalizeModelID(modelID)
	if id == "" {
		return 0, false
	}

	orCacheMu.RLock()
	cacheValid := orModelCache != nil && time.Since(orCacheModTime) < orCacheTTL
	var val int
	var ok bool
	if cacheValid {
		val, ok = findInMap(orModelCache, id)
	}
	orCacheMu.RUnlock()

	if ok && val > 0 {
		return val, true
	}

	if !cacheValid {
		orCacheMu.Lock()
		// Double check both cache staleness and failure backoff
		needsFetch := orModelCache == nil || time.Since(orCacheModTime) >= orCacheTTL
		inBackoff := !orCacheFailTime.IsZero() && time.Since(orCacheFailTime) < orFailBackoff
		if needsFetch && !inBackoff {
			newCache := fetchOpenRouterModels(ctx, apiKey)
			if newCache != nil {
				orModelCache = newCache
				orCacheModTime = time.Now()
				orCacheFailTime = time.Time{}
			} else {
				orCacheFailTime = time.Now()
			}
		}
		if orModelCache != nil {
			val, ok = findInMap(orModelCache, id)
		}
		orCacheMu.Unlock()
	}

	if ok && val > 0 {
		return val, true
	}

	return 0, false
}

func findInMap(cache map[string]int, id string) (int, bool) {
	if val, ok := cache[id]; ok {
		return val, true
	}
	for key, val := range cache {
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.TrimSpace(key[idx+1:])
			if suffix == id {
				return val, true
			}
		}
	}
	return 0, false
}

var openRouterAPIURL = "https://openrouter.ai/api/v1/models"

func fetchOpenRouterModels(ctx context.Context, apiKey string) map[string]int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterAPIURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var parsed openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil
	}

	cache := make(map[string]int)
	for _, m := range parsed.Data {
		normalized := normalizeModelID(m.ID)
		if normalized != "" && m.ContextLength > 0 {
			cache[normalized] = m.ContextLength
		}
	}
	return cache
}
