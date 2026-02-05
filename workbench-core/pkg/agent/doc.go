// Package agent exposes the public API surface for embedding the Workbench agent.
//
// It wraps the internal agent implementation, exposing both the configuration
// required to assemble an agent and the runtime behaviors that hosts depend on.
// This package is intended to be the primary entry point for anyone building a
// custom hosting environment or integrating Workbench flows into another system.
//
// # Core Components
//
//   - AgentConfig: Captures the configuration (model, prompts, hooks)
//     and `NewAgent` returns an `Agent` that can run goals.
//   - HostExecutor: The host primitive dispatcher that receives `types.HostOpRequest`
//     and returns `types.HostOpResponse` values (see `types.HostExecFunc` for a helper).
//   - PromptSource / PromptBuilder: Hooks for inserting step-specific context
//     (file snippets, memories, dynamic prompts) into the agent before it queries the LLM.
//   - Hooks: Callbacks for observing token usage, tool runs, streams, and logging.
//   - HostToolRegistry: Registers builtin tool-call functions (fs_*, shell_exec, http_fetch, trace_run)
//     which map onto host ops (fs_*, shell_exec, http_fetch, browser, trace_run).
//
// # Usage Pattern
//
// Typical usage includes the following steps:
//
//  1. Populate an `agent.AgentConfig` with a validated `llm.LLMClient`, host executor,
//     model name, prompt customization, and optional `PromptSource`.
//  2. Call `agent.NewAgent(llmClient, exec, cfg)` to receive an `Agent`. `NewAgent` performs config validation
//     and wires builtin tools so consumers get a consistent environment.
//  3. Invoke `Agent.Run(ctx, goal)` (or respond to `HostOpFinal`) to execute the
//     agent loop. Hooks let observers subscribe to token usage, streaming text,
//     or tool invocations.
//
// Example snippet:
//
//	cfg := agent.AgentConfig{Model: "openai/gpt-4o"}
//	ag, err := agent.NewAgent(myLLM, types.HostExecFunc(myExec), cfg)
//	if err != nil {
//	    log.Fatalf("unable to create agent: %v", err)
//	}
//
// # Stability and Extension Points
//
// Most of the exported structs (AgentConfig, Hooks, HostToolRegistry, PromptSource) are
// designed to remain stable across patch releases. Hosts hooking into these APIs
// should treat the validation performed by `New` and the built-in tool wiring as
// contract guarantees—changing those behaviors will be accompanied by version bumps.
package agent
