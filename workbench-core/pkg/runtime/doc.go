// Package runtime manages the execution environment that runs a Workbench agent.
//
// It wires together the virtual filesystem (VFS), tool registry, stores, and helpers
// (skills, resources, traces, etc.) so the agent loop can focus on prompting and
// tool orchestration. This package exists to provide a rebuildable `Runtime` that
// any host can instantiate when embedding Workbench functionality.
//
// # Responsibilities
//
//   - VFS Setup: `Build` creates a `vfs.FS`, mounts `/project`, `/workspace`, `/plan`,
//     `/skills`, `/log`, `/history`, and any other resources the
//     runtime needs.
//   - Tool + Skill Wiring: Runtime configures built-in host tools (fs.*, shell, code_exec, http, browser, trace).
//     Skills are discovered from `~/.agents/skills` and mounted under `/skills` so the agent
//     can read their instructions via the VFS.
//   - Context + Hook Integration: The runtime holds `agent.PromptBuilder` and
//     `agent.PromptUpdater` structs along with trace middleware so contextual prompts
//     and reasoning traces flow through each step. It also orchestrates event emission
//     and persistence callbacks supplied via the `BuildConfig` (history, memory,
//     trace stores, etc.).
//   - Guardrails: BuildConfig allows hosts to enforce op guards, artifact observers,
//     run persistence hooks, and session loading/saving so runtime initialization can
//     surface runtime-specific behavior without leaking implementation details.
//
// # Usage Pattern
//
//	cfg := runtime.BuildConfig{...} // provide stores, executors, emit hooks, etc.
//	rt, err := runtime.Build(cfg)
//	if err != nil {
//	    return nil, err
//	}
//	defer rt.Shutdown(ctx)
//
// The `Runtime` exposes a `HostExecutor` that wires into `agent.New`; it can validate
// configuration, mount stores, and register builtin tools so higher layers do not
// need to duplicate these concerns.
package runtime
