// Package config centralizes Agen8-wide configuration values.
//
// Right now the package exposes `DataDir`, the base directory where Agen8
// stores sessions, runs, and runtime artifacts. This directory is resolved
// via CLI flags (e.g., `--data-dir`), environment variables (`AGEN8_DATA_DIR`),
// or defaults (`~/.agen8`).
//
// # Data Layout
//
// Hosts rely on `config.DataDir` containing the following structure:
//
//   - `<dataDir>/sessions/` – metadata about each session (for resuming + listing)
//   - `<dataDir>/agents/<agentId>/` – per-agent artifacts (workspace, log, plan, runtime state)
//   - `<dataDir>/memory/` – shared memory (`MEMORY.MD`, plus daily `YYYY-MM-DD-memory.md` files)
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
// Consumers should treat the `DataDir` contract as stable: Agen8 expects this
// directory tree to exist before wiring artifacts, so hosts should ensure directories
// are created and writable prior to runtime initialization.
//
// # Consumption
//
//   - CLI: `config.DataDir` is populated during CLI initialization via flags such as `--data-dir`.
//   - Environment: `AGEN8_DATA_DIR` can override CLI defaults without modifying code.
//   - Runtime: `runtime.BuildConfig` reads `config.DataDir` before constructing resources and mounts.
//   - Host tooling: hosts may add wrappers that call `config.EnsureDataDir` helpers to guard directory setup.
package config
