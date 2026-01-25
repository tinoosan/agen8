// Package fsutil provides filesystem utilities for workbench data management.
//
// This package contains helper functions for constructing canonical file paths
// and performing atomic file operations. It serves as a single source of truth
// for workbench's data layout conventions.
//
// # Path Construction Helpers
//
// The package provides typed path builders that enforce the standard directory
// structure under DataDir:
//
//	GetSQLitePath(dataDir)                   -> data/workbench.db
//	GetScratchDir(dataDir, runId)            -> data/runs/<runId>/scratch
//	GetResultsDir(dataDir, runId)            -> data/runs/<runId>/results
//	GetLogDir(dataDir, runId)                -> data/runs/<runId>/log
//	GetRunMemoryDir(dataDir, runId)          -> data/runs/<runId>/memory
//	GetToolsDir(dataDir)                     -> data/tools
//	GetToolManifestPath(baseDir, toolId)     -> <baseDir>/<toolId>/manifest.json
//
// # Atomic Write Operations
//
// The package provides WriteFileAtomic which uses a temp-file-plus-rename strategy
// to ensure file writes are atomic at the filesystem level. This prevents partial
// writes from being visible to readers and reduces the risk of corruption.
//
// # Usage
//
//	cfg := config.Default()
//	dbPath := fsutil.GetSQLitePath(cfg.DataDir)
//	_ = dbPath
package fsutil
