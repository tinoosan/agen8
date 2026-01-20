package tui

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const debugLogPath = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"

var debugLogMu sync.Mutex

func debugLog(sessionId, runId, hypothesisId, location, message string, data map[string]interface{}) {
	if sessionId == "" {
		sessionId = "debug-session"
	}
	if runId == "" {
		runId = "pre-fix"
	}
	payload := map[string]interface{}{
		"sessionId":    sessionId,
		"runId":        runId,
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	f, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

