// Package config provides configuration settings for the workbench system.
//
// Currently, config is minimal and contains a single variable DataDir which
// specifies the base directory for all workbench data storage.
//
// # Data Directory Structure
//
// The DataDir (resolved by the CLI via ResolveDataDir) serves as the root for:
//   - <dataDir>/runs/<runId>/         Run-scoped directories
//   - <dataDir>/tools/<toolId>/       Custom tool manifests
//   - <dataDir>/agent/               Agent state (future)
//   - <dataDir>/knowledge/           Knowledge base (future)
//
// # Future Configuration
//
// This package will likely expand to support:
//   - Environment variable overrides
//   - Configuration file loading (YAML/TOML)
//   - Per-run configuration
//   - API keys and credentials
package config
