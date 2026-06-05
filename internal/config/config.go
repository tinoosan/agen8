package config

import (
	"fmt"
	"strings"
)

// Config holds host runtime configuration that should not be global state.
//
// This is intentionally small for now. As more knobs are added, they should live
// here rather than as package-level globals to keep code testable and parallel-safe.
type Config struct {
	// DataDir is the base directory for all agen8 data storage.
	//
	// All run-scoped data (workspace, events, results, etc.) is stored under
	// subdirectories of DataDir.
	//
	// Note: the CLI resolves the default using ResolveDataDir (home/XDG by
	// default, with overrides via --data-dir / AGEN8_DATA_DIR).
	DataDir string

	// DBDriver selects the storage backend. Empty means sqlite for local
	// compatibility. Hosted deployments should set this to postgres.
	DBDriver string

	// DatabaseURL is required when DBDriver is postgres.
	DatabaseURL string
}

// Default returns the default host configuration. Runtime entrypoints resolve
// DataDir through ResolveDataDir so state lives in the user's agen8 home unless
// the caller explicitly overrides it.
func Default() Config {
	return Config{
		DataDir: "",
	}
}

func (c Config) Validate() error {
	if err := nonEmpty("config.DataDir", c.DataDir); err != nil {
		return err
	}
	switch c.DBDriver {
	case "", "sqlite":
	case "postgres":
		if err := nonEmpty("config.DatabaseURL", c.DatabaseURL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("config.DBDriver: unknown driver %q (want %q or %q)", c.DBDriver, "sqlite", "postgres")
	}
	return nil
}

func nonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
