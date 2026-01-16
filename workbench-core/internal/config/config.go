package config

// DataDir is the base directory for all workbench data storage.
//
// All run-scoped data (workspace, events, results, etc.) is stored under
// subdirectories of DataDir. The default value "data" creates a local
// data directory in the current working directory.
//
// This will likely become configurable via environment variable or config file
// in future versions to support deployment scenarios where data should be
// stored in a specific location (e.g., /var/lib/workbench, ~/.workbench, etc.).
var DataDir = "data"
