package config

import "github.com/tinoosan/agen8/pkg/validate"

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

	// CodeExec controls runtime policy for code_exec Python environment setup.
	CodeExec CodeExecConfig
}

// CodeExecConfig controls code_exec Python runtime dependency policy.
type CodeExecConfig struct {
	VenvPath         string
	RequiredPackages []string
}

// Default returns the default host configuration.
func Default() Config {
	return Config{
		DataDir: "db",
		CodeExec: CodeExecConfig{
			VenvPath:         "",
			RequiredPackages: nil,
		},
	}
}

func (c Config) Validate() error {
	if err := validate.NonEmpty("config.DataDir", c.DataDir); err != nil {
		return err
	}
	return nil
}
