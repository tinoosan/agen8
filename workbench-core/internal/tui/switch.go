package tui

import "fmt"

// SwitchSessionError signals a request to switch sessions in-process.
type SwitchSessionError struct {
	SessionID string
	New       bool
}

func (e SwitchSessionError) Error() string {
	if e.New {
		return "switch session: new"
	}
	if e.SessionID != "" {
		return fmt.Sprintf("switch session: %s", e.SessionID)
	}
	return "switch session"
}
