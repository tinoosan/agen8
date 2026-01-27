// Package agent exposes the public API surface for embedding the Workbench agent.
//
// It wraps the internal agent implementation, exposing both the configuration
// required to assemble an agent and the runtime behaviors that hosts depend on.
// This package is intended to be the primary entry point for anyone building a
// custom hosting environment or integrating Workbench flows into another system.
//
// # Core Components
//
//   - Agent and Config: `Config` captures the dependencies (LLM client, host executor,
//     tool manifests, prompts, hooks) and `New` returns an `Agent` that can run goals.
//   - HostExecutor / HostExecFunc: The host primitive dispatcher that receives
//     `types.HostOpRequest` and returns `types.HostOpResponse` values.
//   - ContextSource / ContextConstructor: Hooks for inserting step-specific context
//     (file snippets, memories, dynamic prompts) into the agent before it queries the LLM.
//   - Hooks: Callbacks for observing token usage, tool runs, streams, and logging.
//   - ToolRegistry: Registers builtin tools (fs.*, shell_exec, http_fetch, trace, tool.run)
//     and exposes additional tools provided via Config.ToolManifests.
//
// # Usage Pattern
//
// Typical usage includes the following steps:
//
//   1. Populate an `agent.Config` with a validated `llm.LLMClient`, host executor,
//      model name, prompt customization, and optional `ContextSource`.
//   2. Call `agent.New(cfg)` to receive an `Agent`. `New` performs config validation
//      and wires builtin tools so consumers get a consistent environment.
//   3. Invoke `Agent.Run(ctx, goal)` (or respond to `HostOpFinal`) to execute the
//      agent loop. Hooks let observers subscribe to token usage, streaming text,
//      or tool invocations.
//
// Example snippet:
//
//   cfg := agent.Config{LLM: myLLM, Exec: agent.HostExecFunc(myExec), Model: "openai/gpt-4o"}
//   ag, err := agent.New(cfg)
//   if err != nil {
//       log.Fatalf("unable to create agent: %v", err)
//   }
//
// # Stability and Extension Points
//
// Most of the exported structs (Config, Hooks, ToolRegistry, ContextSource) are
// designed to remain stable across patch releases. Hosts hooking into these APIs
// should treat the validation performed by `New` and the built-in tool wiring as
// contract guarantees—changing those behaviors will be accompanied by version bumps.
// Additional tools exposed through `Config.ToolManifests` are registered with the
// same registry that handles builtin host operations, so there is a single place to
// audit tool availability and runtime routing.
package agent
