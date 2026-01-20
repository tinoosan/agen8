package debuglog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	sessionID = "debug-session"
	logPath   = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"
)

type payload struct {
	SessionID    string         `json:"sessionId"`
	RunID        string         `json:"runId"`
	HypothesisID string         `json:"hypothesisId"`
	Location     string         `json:"location"`
	Message      string         `json:"message"`
	Data         map[string]any `json:"data,omitempty"`
	Timestamp    int64          `json:"timestamp"`
}

func Log(runID, hypothesisID, location, message string, data map[string]any) {
	p := payload{
		SessionID:    sessionID,
		RunID:        runID,
		HypothesisID: hypothesisID,
		Location:     location,
		Message:      message,
		Data:         data,
		Timestamp:    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}
