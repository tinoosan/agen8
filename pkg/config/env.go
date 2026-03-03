package config

import (
	"os"
	"strings"
)

// ParseBoolEnvDefault parses an environment variable as a boolean.
// Empty or unrecognized values return def.
func ParseBoolEnvDefault(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on", "allow":
		return true
	case "0", "false", "no", "off", "deny", "disallow":
		return false
	default:
		return def
	}
}
