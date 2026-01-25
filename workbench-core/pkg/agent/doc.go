// Package agent provides the public API for the Workbench agent.
//
// It wraps the internal agent implementation, exposing key types and configuration
// options necessary for embedding and interacting with the agent. This package
// is intended to be the primary entry point for consumers of the agent functionality.
//
// # Core Components
//
//   - Agent: The main struct that orchestrates the execution loop (Model -> Host -> Model).
//   - Config: Configuration details including the LLM client, host executor, and behavior flags.
//   - HostExecutor: Interface for the environment where the agent operates (handles tools/files).
//   - ContextSource: Mechanism to inject dynamic context (like file contents or memory) into the prompt.
//
// # Usage
//
// To use the agent, create a Config struct with your LLM client and host details,
// then call New(config) to get an Agent instance. Use Agent.Run() to execute a goal.
package agent
