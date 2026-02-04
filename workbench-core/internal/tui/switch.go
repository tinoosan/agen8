package tui

import (
	"fmt"
	"strings"
)

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

// MonitorSwitchRunError is returned by RunMonitor when the user selects a
// different session/run to monitor (e.g. via /sessions).
//
// Callers can intercept this error and restart the monitor for the requested run.
type MonitorSwitchRunError struct {
	RunID string
}

func (e *MonitorSwitchRunError) Error() string {
	return "switch monitor to run: " + strings.TrimSpace(e.RunID)
}
