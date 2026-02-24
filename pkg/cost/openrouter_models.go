package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type openRouterModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		ContextLength int    `json:"context_length"`
		// OpenRouter model metadata may include parameter capability hints.
		SupportedParameters []string `json:"supported_parameters"`
		Pricing             struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
		TopProvider struct {
			ContextLength int `json:"context_length"`
		} `json:"top_provider"`
	} `json:"data"`
}

var (
	orModelCache    map[string]openRouterModelMeta
	orCacheModTime  time.Time
	orCacheFailTime time.Time
	orCacheMu       sync.RWMutex
	orCacheTTL      = 1 * time.Hour
	orFailBackoff   = 2 * time.Minute
)

type openRouterModelMeta struct {
	ContextLength     int
	SupportsReasoning bool
	InputPerM         float64
	OutputPerM        float64
	Provider          string
}

// ContextLengthFromOpenRouter fetches the context length for a given model from OpenRouter.
func ContextLengthFromOpenRouter(ctx context.Context, modelID string) (int, bool) {
	meta, ok := openRouterModelMetaFor(ctx, modelID)
	if !ok || meta.ContextLength <= 0 {
		return 0, false
	}
	return meta.ContextLength, true
}

// SupportsReasoningSummaryFromOpenRouter reports whether OpenRouter metadata
// indicates that a model supports the reasoning parameter family.
// Returns (supports, known). known=false means metadata was unavailable.
func SupportsReasoningSummaryFromOpenRouter(ctx context.Context, modelID string) (bool, bool) {
	meta, ok := openRouterModelMetaFor(ctx, modelID)
	if !ok {
		return false, false
	}
	return meta.SupportsReasoning, true
}

// OpenRouterModelInfos returns model infos fetched from OpenRouter /models.
// The returned bool reports whether fresh or cached OpenRouter metadata was available.
func OpenRouterModelInfos(ctx context.Context) ([]ModelInfo, bool) {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		return nil, false
	}
	cache, ok := openRouterModelCache(ctx)
	if !ok || len(cache) == 0 {
		return nil, false
	}
	out := make([]ModelInfo, 0, len(cache))
	for id, meta := range cache {
		mid := strings.TrimSpace(id)
		if mid == "" {
			continue
		}
		provider := strings.TrimSpace(meta.Provider)
		if provider == "" {
			if idx := strings.Index(mid, "/"); idx > 0 {
				provider = mid[:idx]
			}
		}
		out = append(out, ModelInfo{
			ID:            mid,
			Provider:      provider,
			InputPerM:     meta.InputPerM,
			OutputPerM:    meta.OutputPerM,
			IsReasoning:   meta.SupportsReasoning,
			ContextLength: meta.ContextLength,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out, len(out) > 0
}

func openRouterModelMetaFor(ctx context.Context, modelID string) (openRouterModelMeta, bool) {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		return openRouterModelMeta{}, false
	}
	id := normalizeModelID(modelID)
	if id == "" {
		return openRouterModelMeta{}, false
	}

	cache, ok := openRouterModelCache(ctx)
	if !ok {
		return openRouterModelMeta{}, false
	}
	meta, ok := findInMap(cache, id)
	if ok {
		return meta, true
	}

	return openRouterModelMeta{}, false
}

func openRouterModelCache(ctx context.Context) (map[string]openRouterModelMeta, bool) {
	orCacheMu.RLock()
	cacheValid := orModelCache != nil && time.Since(orCacheModTime) < orCacheTTL
	if cacheValid {
		cache := orModelCache
		orCacheMu.RUnlock()
		return cache, true
	}
	orCacheMu.RUnlock()

	orCacheMu.Lock()
	defer orCacheMu.Unlock()

	cacheValid = orModelCache != nil && time.Since(orCacheModTime) < orCacheTTL
	if cacheValid {
		return orModelCache, true
	}

	inBackoff := !orCacheFailTime.IsZero() && time.Since(orCacheFailTime) < orFailBackoff
	if inBackoff {
		if orModelCache != nil {
			return orModelCache, true
		}
		return nil, false
	}

	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	newCache := fetchOpenRouterModels(ctx, apiKey)
	if newCache == nil {
		orCacheFailTime = time.Now()
		if orModelCache != nil {
			return orModelCache, true
		}
		return nil, false
	}
	orModelCache = newCache
	orCacheModTime = time.Now()
	orCacheFailTime = time.Time{}
	return orModelCache, true
}

func findInMap(cache map[string]openRouterModelMeta, id string) (openRouterModelMeta, bool) {
	if meta, ok := cache[id]; ok {
		return meta, true
	}
	for key, meta := range cache {
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.TrimSpace(key[idx+1:])
			if suffix == id {
				return meta, true
			}
		}
	}
	return openRouterModelMeta{}, false
}

var openRouterAPIURL = "https://openrouter.ai/api/v1/models"

func fetchOpenRouterModels(ctx context.Context, apiKey string) map[string]openRouterModelMeta {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterAPIURL, nil)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

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

	cache := make(map[string]openRouterModelMeta)
	for _, m := range parsed.Data {
		normalized := normalizeModelID(m.ID)
		if normalized != "" {
			contextLength := m.ContextLength
			if contextLength <= 0 {
				contextLength = m.TopProvider.ContextLength
			}
			cache[normalized] = openRouterModelMeta{
				ContextLength:     contextLength,
				SupportsReasoning: supportsReasoningParameter(m.SupportedParameters),
				InputPerM:         parsePerTokenPriceToPerM(m.Pricing.Prompt),
				OutputPerM:        parsePerTokenPriceToPerM(m.Pricing.Completion),
				Provider:          providerFromModelID(normalized),
			}
		}
	}
	return cache
}

func supportsReasoningParameter(params []string) bool {
	for _, p := range params {
		k := strings.ToLower(strings.TrimSpace(p))
		if k == "" {
			continue
		}
		if k == "reasoning" ||
			k == "reasoning.effort" ||
			strings.HasPrefix(k, "reasoning_") ||
			strings.HasPrefix(k, "reasoning.") {
			return true
		}
	}
	return false
}

func parsePerTokenPriceToPerM(raw string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v <= 0 {
		return 0
	}
	return v * 1_000_000
}

func providerFromModelID(id string) string {
	id = strings.TrimSpace(id)
	if idx := strings.Index(id, "/"); idx > 0 {
		return strings.TrimSpace(id[:idx])
	}
	return ""
}
