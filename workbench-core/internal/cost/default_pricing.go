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
			// GPT-5.2 (400K) pricing provided by the user:
			//   - $1.75 / 1M input tokens
			//   - $14.00 / 1M output tokens
			"openai/gpt-5.2": {
				InputPerM:  1.75,
				OutputPerM: 14.0,
			},
			// GPT-5.2 Chat (128K).
			"openai/gpt-5.2-chat": {
				InputPerM:  1.75,
				OutputPerM: 14.0,
			},
			// GPT-5.2 Pro (400K).
			"openai/gpt-5.2-pro": {
				InputPerM:  21.0,
				OutputPerM: 168.0,
			},

			// GPT-5.1 family (400K unless otherwise noted).
			"openai/gpt-5.1": {
				InputPerM:  1.25,
				OutputPerM: 10.0,
			},
			// GPT-5.1 Chat (128K).
			"openai/gpt-5.1-chat": {
				InputPerM:  1.25,
				OutputPerM: 10.0,
			},
			"openai/gpt-5.1-codex": {
				InputPerM:  1.25,
				OutputPerM: 10.0,
			},
			"openai/gpt-5.1-codex-max": {
				InputPerM:  1.25,
				OutputPerM: 10.0,
			},
			"openai/gpt-5.1-codex-mini": {
				InputPerM:  0.25,
				OutputPerM: 2.0,
			},
		},
	}
}
