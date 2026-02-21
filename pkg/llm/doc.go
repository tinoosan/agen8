// Package llm provides the Large Language Model (LLM) client abstraction for Agen8.
//
// It defines the contract that any LLM provider must satisfy so the agent can
// generate completions, reason about tool usage, and consume streaming tokens in a
// unified way.
//
// # Key Components
//
//   - `LLMClient`: Interface for issuing completions (streaming and non-streaming),
//     requesting tool calls, and reporting usage information.
//   - `LLMRequest`: Captures model inputs (messages, functions, max tokens, tool list,
//     system prompts) so runtime components can build consistent prompts.
//   - `LLMResponse` / `LLMStreamChunk`: Represent the model output, including text
//     tokens, tool call instructions, and usage metrics such as token counts.
//
// # Typical Usage
//
//   1. Provide an `LLMClient` implementation (e.g., `openai.Client`) that talks to a
//      specific provider and satisfies the interface.
//   2. Build `LLMRequest` values via helper constructors and pass them to `LLMClient`.
//   3. Use the returned `LLMResponse` or stream of `LLMStreamChunk` values to react
//      to tool call requests, final answers, or reasoning summaries.
//
// Examples of LLM providers live under `pkg/llm` (e.g., `openai_client.go`), but the
// rest of the runtime code should only depend on the interface so providers can be
// swapped without touching the agent logic.
//
// # Stability
//
// The LLM contract is foundational to Agen8, so most exported interfaces in this
// package are treated as stable. If an interface needs to change (e.g., to expose
// new meta fields), it should be introduced with a clear upgrade path or version
// guard. Clients should rely on the asynchronous streaming helpers when they need to
// interleave tool calls with partial responses.
package llm
