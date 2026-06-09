package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tinoosan/agen8/internal/config"
	"github.com/tinoosan/agen8/internal/logging"
)

const (
	ListenerHTTP = "http"

	EnvListener  = "AGEN8_DAEMON_LISTENER"
	EnvEndpoint  = "AGEN8_RPC_ENDPOINT"
	EnvHTTPAddr  = "AGEN8_HTTP_ADDR"
	EnvDevWebURL = "AGEN8_DEV_WEB_URL"

	DefaultHTTPAddr = "127.0.0.1:7777"
)

type Config struct {
	AppConfig  config.Config
	Logging    logging.Config
	Listener   string
	HTTPAddr   string
	SetupToken string
	Out        io.Writer
}

func (c Config) withDefaults() (Config, error) {
	if c.Out == nil {
		c.Out = io.Discard
	}
	c.Listener = firstNonEmpty(c.Listener, os.Getenv(EnvListener), ListenerHTTP)
	c.HTTPAddr = firstNonEmpty(c.HTTPAddr, os.Getenv(EnvHTTPAddr), DefaultHTTPAddr)
	c.Listener = strings.TrimSpace(strings.ToLower(c.Listener))
	if c.AppConfig.DataDir == "" {
		c.AppConfig = config.Default()
	}
	if strings.TrimSpace(c.AppConfig.DataDir) == "" || strings.TrimSpace(c.AppConfig.DataDir) == "db" {
		dataDir, err := config.ResolveDataDir("", false)
		if err != nil {
			return c, err
		}
		c.AppConfig.DataDir = dataDir
	}
	if err := c.AppConfig.Validate(); err != nil {
		return c, err
	}
	switch c.Listener {
	case ListenerHTTP:
		if strings.TrimSpace(c.HTTPAddr) == "" {
			return c, fmt.Errorf("daemon http address is required")
		}
	default:
		return c, fmt.Errorf("unknown daemon listener %q", c.Listener)
	}
	if strings.TrimSpace(c.SetupToken) == "" {
		token, err := randomToken(32)
		if err != nil {
			return c, err
		}
		c.SetupToken = token
	}
	return c, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func randomToken(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("token byte length must be positive")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate setup token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
