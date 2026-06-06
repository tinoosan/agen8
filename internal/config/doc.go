// Package config centralizes Agen8-wide configuration values.
//
// Right now the package exposes `DataDir`, the base directory where Agen8
// stores local daemon state and runtime artifacts. This directory is resolved
// via CLI flags (e.g., `--data-dir`), environment variables (`AGEN8_DATA_DIR`),
// or defaults (`~/.agen8`).
//
// # Data Layout
//
// Hosts rely on `config.DataDir` containing local process state outside the
// project repository:
//
//   - `<dataDir>/agen8.db` – primary SQLite store for users, projects,
//     members, missions, key results, tasks, decisions, graph links,
//     credentials, and files.
//   - `<dataDir>/logs/` – optional daemon logs when file logging is enabled.
//   - `<dataDir>/bridge-sessions/` – local MCP bridge session references when
//     the stdio bridge is used.
//
// # Extension Points
//
// This package is intentionally lightweight today, but it is the logical place to
// add future configuration capabilities, including:
//   - Configuration file loading (TOML/YAML) that overrides defaults.
//   - Per-command overrides supplied by the daemon or bridge.
//   - Credential injection for tool access or LLM providers.
//
// Consumers should treat the `DataDir` contract as stable: Agen8 expects this
// directory tree to exist before opening storage, so hosts should ensure it is
// created and writable prior to daemon initialization.
//
// # Consumption
//
//   - CLI: `config.DataDir` is populated during CLI initialization via flags such as `--data-dir`.
//   - Environment: `AGEN8_DATA_DIR` can override CLI defaults without modifying code.
//   - Host tooling: hosts may add wrappers that call `config.EnsureDataDir` helpers to guard directory setup.
package config
