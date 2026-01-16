package cost

// DefaultPricing returns the built-in pricing table shipped with Workbench.
//
// This keeps the binary self-contained (no required pricing.json file).
//
// To add or update prices, edit this map. Keys should match the model id you pass
// to the provider (e.g. OPENROUTER_MODEL like "openai/gpt-5.2").
func DefaultPricing() PricingFile {
	return PricingFile{
		Version:  "v1",
		Currency: "USD",
		Models: map[string]ModelPricing{
			// GPT-5.2 pricing provided by the user:
			//   - $1.75 / 1M input tokens
			//   - $14.00 / 1M output tokens
			"openai/gpt-5.2": {
				InputPerM:  1.75,
				OutputPerM: 14.0,
			},
			// Keep a conservative default for mini variants until you add the real price.
			"openai/gpt-5-mini": {
				InputPerM:  1.75,
				OutputPerM: 14.0,
			},
		},
	}
}
