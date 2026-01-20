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
			"openai/gpt-5-mini": {
				InputPerM:  0.25,
				OutputPerM: 2.0,
			},
			"openai/gpt-5-nano": {
				InputPerM:  0.05,
				OutputPerM: 0.40,
			},

			// OpenAI (Real).
			"openai/gpt-4o": {
				InputPerM:  2.50,
				OutputPerM: 10.00,
			},
			"openai/gpt-4o-mini": {
				InputPerM:  0.15,
				OutputPerM: 0.60,
			},
			"openai/o1-preview": {
				InputPerM:  15.00,
				OutputPerM: 60.00,
			},
			"openai/o1-mini": {
				InputPerM:  3.00,
				OutputPerM: 12.00,
			},

			// Anthropic.
			"anthropic/claude-3.5-sonnet": {
				InputPerM:  3.00,
				OutputPerM: 15.00,
			},
			"anthropic/claude-3-opus": {
				InputPerM:  15.00,
				OutputPerM: 75.00,
			},
			"anthropic/claude-3-haiku": {
				InputPerM:  0.25,
				OutputPerM: 1.25,
			},
			"anthropic/claude-4.5-opus": {
				InputPerM:  5.00,
				OutputPerM: 25.00,
			},
			"anthropic/claude-4.5-sonnet": {
				InputPerM:  1.00,
				OutputPerM: 15.00,
			},

			// Google.
			"google/gemini-pro-1.5": {
				InputPerM:  2.50,
				OutputPerM: 7.50,
			},
			"google/gemini-flash-1.5": {
				InputPerM:  0.075,
				OutputPerM: 0.30,
			},

			// Meta.
			"meta-llama/llama-3.1-405b-instruct": {
				InputPerM:  2.70,
				OutputPerM: 2.70,
			},
			"meta-llama/llama-3.1-70b-instruct": {
				InputPerM:  0.35,
				OutputPerM: 0.40,
			},
			"meta-llama/llama-3.2-11b-vision-instruct": {
				InputPerM:  0.055,
				OutputPerM: 0.055,
			},
			"meta-llama/llama-3.2-3b-instruct": {
				InputPerM:  0.04, // Often free/cheap, setting conservative
				OutputPerM: 0.04,
			},

			// Mistral.
			"mistralai/mistral-large": {
				InputPerM:  2.00,
				OutputPerM: 6.00,
			},

			// Z.AI.
			"z-ai/glm-4.7": {
				InputPerM:  0.40,
				OutputPerM: 1.50,
			},

			// DeepSeek.
			"deepseek/deepseek-chat": {
				InputPerM:  0.14,
				OutputPerM: 0.28,
			},
			"deepseek/deepseek-r1": {
				InputPerM:  0.55,
				OutputPerM: 2.19, // R1 pricing varies, usually higher than V3
			},

			// Qwen.
			"qwen/qwen-2.5-72b-instruct": {
				InputPerM:  0.35,
				OutputPerM: 0.40,
			},
			"qwen/qwen-2.5-coder-32b-instruct": {
				InputPerM:  0.07,
				OutputPerM: 0.16,
			},
		},
	}
}
