// Package llm provides the LLM client abstraction for workbench.
//
// This package defines the interface for communicating with Large Language Models
// and provides concrete implementations for specific LLM providers.
//
// # Client Interface
//
// The core abstraction is the Client interface, which defines a standard
// request/response contract regardless of the underlying LLM provider:
//
//	type Client interface {
//	    SendMessage(ctx context.Context, req LLMRequest) (LLMResponse, error)
//	}
//
// # Current Implementation
//
// OpenRouterClient is the default implementation, which routes requests through
// the OpenRouter API (https://openrouter.ai). This provides access to multiple
// model providers (OpenAI, Anthropic, etc.) through a single API.
//
// # Request/Response Contract
//
//   - LLMRequest: Contains model ID, messages, tools, and constraints
//   - LLMResponse: Contains the assistant's message and token usage
//   - Messages: Use types.Message (role + content)
//   - Tools: Use types.Tool for function calling schemas
//
// # Authentication
//
// The OpenRouter client expects an API key via environment variable:
//
//	export OPENROUTER_API_KEY=sk-or-v1-...
//
// # Error Handling
//
// LLM errors are returned as Go errors. Callers should check for:
//   - Network failures
//   - Authentication errors (401)
//   - Rate limiting (429)
//   - Model-specific errors (context length exceeded, etc.)
package llm
