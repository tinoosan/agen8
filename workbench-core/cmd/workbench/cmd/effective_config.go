package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tinoosan/workbench-core/pkg/config"
)

func effectiveConfig(cmd *cobra.Command) (config.Config, error) {
	cliWasSet := false
	if cmd != nil {
		cliWasSet = cmd.Root().PersistentFlags().Changed("data-dir")
	}
	resolved, err := config.ResolveDataDir(dataDir, cliWasSet)
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
	return cfg, nil
}
