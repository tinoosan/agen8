// Package config provides configuration settings for the workbench system.
//
// Currently, config is minimal and contains a single variable DataDir which
// specifies the base directory for all workbench data storage.
//
// # Data Directory Structure
//
// The DataDir (default: "data") serves as the root for:
//   - data/runs/<runId>/           Run-scoped directories
//   - data/tools/<toolId>/          Custom tool manifests
//   - data/agent/                   Agent state (future)
//   - data/knowledge/               Knowledge base (future)
//
// # Future Configuration
//
// This package will likely expand to support:
//   - Environment variable overrides
//   - Configuration file loading (YAML/TOML)
//   - Per-run configuration
//   - API keys and credentials
package config
