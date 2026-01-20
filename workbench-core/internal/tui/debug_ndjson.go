package tui

import (
	"encoding/json"
	"os"
	"time"
)

const debugLogPath = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"

type debugLogPayload struct {
	SessionID    string         `json:"sessionId"`
	RunID        string         `json:"runId"`
	HypothesisID string         `json:"hypothesisId"`
	Location     string         `json:"location"`
	Message      string         `json:"message"`
	Data         map[string]any `json:"data,omitempty"`
	Timestamp    int64          `json:"timestamp"`
}

func debugLog(p debugLogPayload) {
	p.Timestamp = time.Now().UnixMilli()
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	f, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

