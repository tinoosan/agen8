// Package tools implements the tool management system for the Workbench runtime.
//
// It provides the infrastructure for discovering, registering, and executing tools.
// This encompasses both builtin host operations (e.g., filesystem helpers, shell
// commands, HTTP fetching) and any guest-provided manifests stored under `/tools`.
//
// # Key Concepts
//
//   - Registry: Holds `ToolInvoker` implementations keyed by tool ID so runners can
//     locate and call tools at runtime.
//   - Manifest: The schema (`ToolManifest`, `ToolAction`) that describes inputs,
//     outputs, and metadata for a tool. Builtin tools validate their manifest before
//     registration, and hosts can register additional `Config.ToolManifests` through
//     the agent surface.
//   - Orchestrator: Orchestrates calls to tools, validates requests (`ToolRequest`), handles
//     timeouts, persists results via `ResultWriter`, and normalizes errors.
//   - Invoker: The `ToolInvoker` interface abstracts the execution of a tool action,
//     allowing builtin and custom tools to plug into the same runner.
//
// # Runtime Flow
//
//  1. Hosts register tools with a `ToolRegistry` implementation (`MapRegistry` is
//     useful for tests). Builtins use `Register` helpers so they can be verified
//     before being exposed to the agent.
//  2. When the agent issues a `tool.run` host operation, the `Orchestrator` validates the
//     `ToolRequest`, enforces timeouts, and fetches the invoker from the registry.
//  3. The invoker returns `ToolCallResult` with optional artifacts. The runner
//     writes the artifacts and the final `ToolResponse` via the `ResultWriter` so
//     the calling host/agent can inspect the JSON response later.
//  4. Errors are normalized into `ToolResponseError` values with retryable hints.
//
// # Usage Example
//
//	registry := tools.MapRegistry{}
//	registry[toolID] = myInvoker
//	orchestrator := &tools.Orchestrator{Results: resultsWriter, ToolRegistry: registry}
//	resp, err := orchestrator.Run(ctx, toolID, actionID, inputSchema, timeoutMs)
//
// # Stability and Extension
//
// The packages under this directory expose the stable, embeddable tool execution
// surface for both builtin host operations and custom tools. Most types (e.g. `Orchestrator`,
// `ToolManifest`, `ToolRequest`, `ToolResponse`) are intended to remain stable across
// patch versions; changes that affect the tool protocol will carry version bumps or
// compatibility notes. Hosts should rely on the validation helpers (e.g., `validateAndCleanArtifactWrite`)
// to enforce consistent inputs/outputs when building custom invokers.
package tools
