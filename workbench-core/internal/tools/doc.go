// Package tools provides the execution layer for Workbench tools.
//
// # Tiered Tool Architecture
//
// Workbench uses a two-tier tool architecture that separates always-available
// core primitives from discoverable extended tools:
//
//	┌─────────────────────────────────────────────────────────────────────────┐
//	│                         Model Function Calls                            │
//	└───────────────────────────────┬─────────────────────────────────────────┘
//	                                │
//	                                ▼
//	┌─────────────────────────────────────────────────────────────────────────┐
//	│                         Host Loop (loop.go)                             │
//	│                        functionCallToHostOp                             │
//	└───────────┬─────────────────────────────────────┬───────────────────────┘
//	            │                                     │
//	            ▼                                     ▼
//	┌───────────────────────────────┐   ┌─────────────────────────────────────┐
//	│  Tier 1: Core Host Operations │   │      Tier 2: Extended Tools         │
//	│  (Always Available)           │   │      (Require Discovery)            │
//	├───────────────────────────────┤   ├─────────────────────────────────────┤
//	│  • fs_list, fs_read, fs_write │   │  • tool_run → /tools discovery      │
//	│  • fs_append, fs_edit, fs_patch│   │  • Custom tools (YAML/JSON config) │
//	│  • shell_exec                 │   │  • MCP server tools                 │
//	│  • http_fetch                 │   │  • Container/script adapters        │
//	│  • trace                      │   │                                     │
//	│  • final_answer               │   │                                     │
//	└───────────────────────────────┘   └─────────────────────────────────────┘
//
// # Tier 1: Core Host Operations
//
// Core operations are native function calls always available to the model.
// They require no discovery and are handled directly by the host loop:
//
//   - File System: fs_list, fs_read, fs_write, fs_append, fs_edit, fs_patch
//   - Shell: shell_exec — execute commands in the project directory
//   - HTTP: http_fetch — make HTTP requests
//   - Context: trace — write/read reasoning traces across turns
//   - Control: final_answer — end the turn with user-visible response
//
// These are defined in [agent.HostOpFunctions] and routed in [agent.functionCallToHostOp].
//
// # Tier 2: Extended/Discoverable Tools
//
// Extended tools require discovery via /tools and invocation via tool_run:
//
//  1. Model calls fs_list("/tools") to discover available tools
//  2. Model calls fs_read("/tools/<toolId>") to read the manifest
//  3. Model calls tool_run(toolId, actionId, input) to invoke
//
// This tier is for modular capabilities: custom tools, MCP servers, adapters.
//
// # Key Types
//
//   - [Runner] executes tool calls and persists results to /results/<callId>/
//   - [ToolRegistry] looks up tools by ID for execution
//   - [ToolInvoker] is the interface all tool implementations satisfy
//   - [BuiltinConfig] configures built-in invokers (shell root, confirmation, etc.)
//
// # Package Layout
//
//   - tool_runner.go: Runner lifecycle, validation, persistence
//   - builtins_registry.go: Registration for discoverable builtins
//   - builtin_shell.go: Shell execution invoker (used by core shell_exec)
//   - builtin_http.go: HTTP fetch invoker (used by core http_fetch)
//   - builtin_trace.go: Trace store invoker (used by core trace)
//   - manifest_registry.go: Manifest providers for /tools discovery
package tools
