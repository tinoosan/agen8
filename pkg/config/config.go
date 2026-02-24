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

	// PathAccess controls which paths outside VFS the agent may access.
	PathAccess PathAccessConfig

	// SkillsUnmappedRoleFallbackToProfile, when true, allows team runs whose role
	// is not found in the profile to fall back to profile-level skills instead of
	// fail-closed (zero skills). Default false for secure-by-default behavior.
	SkillsUnmappedRoleFallbackToProfile bool
}

// CodeExecConfig controls code_exec Python runtime dependency policy.
type CodeExecConfig struct {
	VenvPath         string
	RequiredPackages []string
}

// PathAccessConfig controls path allowlist for code_exec (dirs outside VFS).
// Paths are on the filesystem where the config lives (client in project mode, daemon host in data-dir mode).
type PathAccessConfig struct {
	Allowlist []string // Absolute dir paths the agent may access outside VFS
	ReadOnly  bool     // If true, only reads allowed; if false, reads and writes
}

// Default returns the default host configuration.
func Default() Config {
	return Config{
		DataDir: "db",
		CodeExec: CodeExecConfig{
			VenvPath:         "",
			RequiredPackages: nil,
		},
		PathAccess: PathAccessConfig{
			Allowlist: nil,
			ReadOnly:  true,
		},
	}
}

func (c Config) Validate() error {
	if err := validate.NonEmpty("config.DataDir", c.DataDir); err != nil {
		return err
	}
	return nil
}
