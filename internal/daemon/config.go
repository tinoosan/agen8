package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
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
	EnvPublicURL = "AGEN8_PUBLIC_URL"
	// #nosec G101 -- this is the setup token environment variable name, not a token value.
	EnvSetupToken = "AGEN8_SETUP_TOKEN"

	EnvDisableLocalHookProvisioning = "AGEN8_DISABLE_LOCAL_HOOK_PROVISIONING"

	DefaultHTTPAddr = "127.0.0.1:7777"
)

type Config struct {
	AppConfig                    config.Config
	Logging                      logging.Config
	Listener                     string
	HTTPAddr                     string
	PublicURL                    string
	DisableLocalHookProvisioning bool
	SetupToken                   string
	Out                          io.Writer
}

func (c Config) withDefaults() (Config, error) {
	if c.Out == nil {
		c.Out = io.Discard
	}
	c.Listener = firstNonEmpty(c.Listener, os.Getenv(EnvListener), ListenerHTTP)
	c.HTTPAddr = firstNonEmpty(c.HTTPAddr, os.Getenv(EnvHTTPAddr), DefaultHTTPAddr)
	c.PublicURL = firstNonEmpty(c.PublicURL, os.Getenv(EnvPublicURL))
	c.SetupToken = firstNonEmpty(c.SetupToken, os.Getenv(EnvSetupToken))
	c.DisableLocalHookProvisioning = c.DisableLocalHookProvisioning || config.ParseBoolEnvDefault(EnvDisableLocalHookProvisioning, false)
	c.Listener = strings.TrimSpace(strings.ToLower(c.Listener))
	c.PublicURL = strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
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
	if c.PublicURL != "" {
		parsed, err := url.Parse(c.PublicURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return c, fmt.Errorf("%s must be an absolute http(s) URL", EnvPublicURL)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return c, fmt.Errorf("%s must use http or https", EnvPublicURL)
		}
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
