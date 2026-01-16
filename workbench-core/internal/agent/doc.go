// Package agent implements the autonomous agent execution loop for workbench.
//
// The agent package orchestrates the core interaction cycle between the user,
// the LLM, and the environment (tools, filesystem, etc.). It manages context,
// evaluates memory updates, and coordinates host operations.
//
// # Agent Loop Architecture
//
// The agent loop is the main execution flow:
//
//  1. User provides a goal
//  2. Agent reads current context (workspace, memory, recent events)
//  3. Agent sends context to LLM
//  4. LLM responds with actions (tool calls, file writes, etc.)
//  5. Host executes actions and records results
//  6. Agent evaluates and commits memory updates
//  7. Loop continues until goal is achieved or user stops
//
// # Key Components
//
//   - Loop: Main agent execution coordinator
//   - ContextUpdater: Manages agent context size and memory
//   - MemoryEvaluator: Processes memory updates from agent
//   - HostOps: Interface for environment interactions (mocked in tests)
//
// # Context Management
//
// The agent maintains a context budget (MaxBytesForContext) to fit within LLM
// token limits. The ContextUpdater dynamically selects what to include based on
// relevance and recency.
//
// # Memory Lifecycle
//
//  1. Agent writes to /memory/update.md
//  2. MemoryEvaluator reads and validates the update
//  3. Update is appended to /memory/memory.md
//  4. update.md is cleared for next turn
//  5. memory.md is injected into system prompt
package agent
