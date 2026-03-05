package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/pkg/config"
)

func effectiveConfig(cmd *cobra.Command) (config.Config, error) {
	cliWasSet := false
	if cmd != nil {
		cliWasSet = cmd.Root().PersistentFlags().Changed("data-dir")
	}
	dataDirValue := dataDir
	if !cliWasSet && strings.TrimSpace(os.Getenv(config.EnvDataDir)) == "" {
		if projectCtx, err := app.LoadProjectContext(projectSearchDir()); err == nil && projectCtx.Exists {
			if override := strings.TrimSpace(projectCtx.Config.DataDirOverride); override != "" {
				dataDirValue = override
				cliWasSet = true // treat as explicit value for resolver validation.
			}
		}
	}

	resolved, err := config.ResolveDataDir(dataDirValue, cliWasSet)
	if err != nil {
		if cliWasSet {
			return config.Config{}, err
		}
		// Provide a little extra context for default/env failures.
		return config.Config{}, fmt.Errorf("resolve data dir: %w", err)
	}
	cfg := config.Config{DataDir: strings.TrimSpace(resolved)}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	if err := app.ApplyRuntimeConfigEnvDefaults(cfg.DataDir); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}
