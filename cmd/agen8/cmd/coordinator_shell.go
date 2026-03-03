package cmd

import "strings"

func isRetryableLiveError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "connection refused"):
		return true
	case strings.Contains(msg, "broken pipe"):
		return true
	case strings.Contains(msg, "reset by peer"):
		return true
	case strings.Contains(msg, "i/o timeout"):
		return true
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "eof"):
		return true
	case strings.Contains(msg, "thread not found"):
		return true
	default:
		return false
	}
}
