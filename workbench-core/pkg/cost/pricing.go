package cost

import (
	"strings"
)

type PricingFile struct {
	Version  string                  `json:"version"`
	Currency string                  `json:"currency"`
	Models   map[string]ModelPricing `json:"models"`
}

type ModelPricing struct {
	InputPerM  float64 `json:"inputPerM"`
	OutputPerM float64 `json:"outputPerM"`
}

func (pf PricingFile) Lookup(model string) (inPerM, outPerM float64, ok bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return 0, 0, false
	}
	if p, ok := pf.Models[model]; ok {
		return p.InputPerM, p.OutputPerM, true
	}
	lower := strings.ToLower(model)
	for k, p := range pf.Models {
		if strings.ToLower(strings.TrimSpace(k)) == lower {
			return p.InputPerM, p.OutputPerM, true
		}
	}
	for k, p := range pf.Models {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if idx := strings.LastIndex(key, "/"); idx >= 0 && idx+1 < len(key) {
			suffix := strings.ToLower(strings.TrimSpace(key[idx+1:]))
			if suffix == lower {
				return p.InputPerM, p.OutputPerM, true
			}
		}
	}
	return 0, 0, false
}
