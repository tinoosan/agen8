// Package config centralizes Workbench-wide configuration values.
//
// Right now the package exposes `DataDir`, the base directory where Workbench
// stores sessions, runs, tools, and runtime artifacts. This directory is resolved
// via CLI flags (e.g., `--data-dir`), environment variables (`WORKBENCH_DATA_DIR`),
// or defaults (`~/.workbench`).
//
// # Data Layout
//
// Hosts rely on `config.DataDir` containing the following structure:
//
//   - `<dataDir>/sessions/` – metadata about each session (for resuming + listing)
//   - `<dataDir>/agents/<agentId>/` – per-agent artifacts (workspace, results, traces)
//   - `<dataDir>/memory/` – shared memory staging (`memory.md`, `update.md`, `commits.jsonl`)
//   - `<dataDir>/tools/` – discovered tool manifests (custom tools and builtin stubs)
//   - `<dataDir>/agent/`, `/knowledge/` – upcoming runtime stores that will continue to live under `DataDir`.
//
// # Extension Points
//
// This package is intentionally lightweight today, but it is the logical place to
// add future configuration capabilities, including:
//   - Configuration file loading (TOML/YAML) that overrides defaults.
//   - Per-run overrides that may come from `Runtime.BuildConfig`.
//   - Credential injection for tool access or LLM providers.
//
// Consumers should treat the `DataDir` contract as stable: Workbench expects this
// directory tree to exist before wiring artifacts, so hosts should ensure directories
// are created and writable prior to runtime initialization.
//
// # Consumption
//
//   - CLI: `config.DataDir` is populated during CLI initialization via flags such as `--data-dir`.
//   - Environment: `WORKBENCH_DATA_DIR` can override CLI defaults without modifying code.
//   - Runtime: `runtime.BuildConfig` reads `config.DataDir` before constructing resources and mounts.
//   - Host tooling: hosts may add wrappers that call `config.EnsureDataDir` helpers to guard directory setup.
package config
