package config

import (
	"github.com/tinoosan/workbench-core/internal/validate"
)

// Config holds host runtime configuration that should not be global state.
//
// This is intentionally small for now. As more knobs are added, they should live
// here rather than as package-level globals to keep code testable and parallel-safe.
type Config struct {
	// DataDir is the base directory for all workbench data storage.
	//
	// All run-scoped data (workspace, events, results, etc.) is stored under
	// subdirectories of DataDir. The default value "data" creates a local
	// data directory in the current working directory.
	DataDir string
}

// Default returns the default host configuration.
func Default() Config {
	return Config{DataDir: "data"}
}

func (c Config) Validate() error {
	if err := validate.NonEmpty("config.DataDir", c.DataDir); err != nil {
		return err
	}
	return nil
}
