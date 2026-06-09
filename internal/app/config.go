package app

import (
	"fmt"

	hostconfig "github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/logging"
)

type Config struct {
	Host           hostconfig.Config
	Logging        logging.Config
	DaemonHTTPAddr string
}

func (c Config) Validate() error {
	if err := c.Host.Validate(); err != nil {
		return err
	}
	switch c.Logging.Level {
	case "", "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("app logging level: unknown level %q (want debug, info, warn, or error)", c.Logging.Level)
	}
}
