package cost

import "context"

// LookupPricing resolves pricing for a model from the static registry first,
// then falls back to OpenRouter /models metadata when available.
func LookupPricing(ctx context.Context, modelID string) (inPerM, outPerM float64, ok bool) {
	if in, out, known := DefaultPricing().Lookup(modelID); known {
		return in, out, true
	}
	infos, known := OpenRouterModelInfos(ctx)
	if !known || len(infos) == 0 {
		return 0, 0, false
	}
	models := make(map[string]ModelPricing, len(infos))
	for _, info := range infos {
		if info.ID == "" {
			continue
		}
		// Keep zero-price entries so free models are still considered "known".
		models[info.ID] = ModelPricing{
			InputPerM:  info.InputPerM,
			OutputPerM: info.OutputPerM,
		}
	}
	pf := PricingFile{Models: models}
	return pf.Lookup(modelID)
}
