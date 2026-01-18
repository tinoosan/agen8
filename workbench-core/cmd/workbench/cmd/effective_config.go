package cmd

import (
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
)

func effectiveConfig() (config.Config, error) {
	cfg := config.Default()
	if strings.TrimSpace(dataDir) != "" {
		cfg.DataDir = strings.TrimSpace(dataDir)
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

