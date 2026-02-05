package tui

import "strings"

func getCostUSD(data map[string]string) string {
	if data == nil {
		return ""
	}
	if v := strings.TrimSpace(data["costUSD"]); v != "" {
		return v
	}
	return strings.TrimSpace(data["costUsd"])
}
