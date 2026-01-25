// Package llm provides the Large Language Model (LLM) client abstraction.
//
// It defines a common interface (`LLMClient`) that allows the workbench to interact
// with various LLM providers (e.g., OpenAI, Anthropic, OpenRouter) in a unified way.
//
// # Key Components
//
//   - LLMClient: Interface for generating completions (streaming and non-streaming).
//   - LLMRequest: Structure containing inputs for the model (messages, tools, etc.).
//   - LLMResponse: Structure containing the model's output (text, tool calls, usage).
//   - Providers: Implementations of LLMClient for specific backends.
package llm
