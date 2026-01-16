package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PricingFile is a simple model->price table used for cost estimation.
//
// The Workbench host uses model-reported token usage and multiplies by the configured
// USD-per-1M prices to produce an estimate. This is an estimate because:
//   - token usage is provider/model reported
//   - billing policies can differ (tool tokens, rounding, cached tokens, etc)
//
// This file is intentionally boring (no wildcards, no inheritance) so it is easy
// to edit by hand.
type PricingFile struct {
	Version  string                  `json:"version"`
	Currency string                  `json:"currency"`
	Models   map[string]ModelPricing `json:"models"`
}

// ModelPricing stores USD per 1M tokens for a model.
type ModelPricing struct {
	InputPerM  float64 `json:"inputPerM"`
	OutputPerM float64 `json:"outputPerM"`
}

// LoadPricingFile reads a pricing JSON file.
//
// If the file does not exist, it returns (PricingFile{}, os.ErrNotExist).
func LoadPricingFile(path string) (PricingFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return PricingFile{}, fmt.Errorf("pricing file path is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return PricingFile{}, err
	}
	var pf PricingFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return PricingFile{}, fmt.Errorf("parse pricing file %s: %w", filepath.Base(path), err)
	}
	if pf.Models == nil {
		pf.Models = map[string]ModelPricing{}
	}
	return pf, nil
}

// Lookup returns the pricing for a model id.
//
// This uses exact match first, then case-insensitive match.
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
	return 0, 0, false
}
