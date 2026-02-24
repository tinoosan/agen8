package cost

import (
	"context"
	"os"
	"strings"
)

// LookupPricing resolves pricing for a model from the static registry first,
// then falls back to OpenRouter /models metadata when available.
func LookupPricing(ctx context.Context, modelID string) (inPerM, outPerM float64, ok bool) {
	if in, out, known := DefaultPricing().Lookup(modelID); known {
		return in, out, true
	}
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		return 0, 0, false
	}
	cache, known := openRouterModelCache(ctx)
	if !known || len(cache) == 0 {
		return 0, 0, false
	}
	models := make(map[string]ModelPricing, len(cache))
	for id, meta := range cache {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if !meta.InputPerMKnown || !meta.OutputPerMKnown {
			continue
		}
		models[id] = ModelPricing{
			InputPerM:  meta.InputPerM,
			OutputPerM: meta.OutputPerM,
		}
	}
	if len(models) == 0 {
		return 0, 0, false
	}
	pf := PricingFile{Models: models}
	return pf.Lookup(modelID)
}
