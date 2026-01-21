package cost

// DefaultPricing returns the built-in pricing table shipped with Workbench.
//
// This keeps the binary self-contained (no required pricing.json file).
//
// To add or update prices, edit `modelInfos`. Keys should match the model id you pass
// to the provider (e.g. OPENROUTER_MODEL like "openai/gpt-5.2").
func DefaultPricing() PricingFile {
	models := make(map[string]ModelPricing, len(modelInfos))
	for _, info := range modelInfos {
		if info.InputPerM == 0 && info.OutputPerM == 0 {
			continue
		}
		models[info.ID] = ModelPricing{
			InputPerM:  info.InputPerM,
			OutputPerM: info.OutputPerM,
		}
	}

	return PricingFile{
		Version:  "v1",
		Currency: "USD",
		Models:   models,
	}
}
