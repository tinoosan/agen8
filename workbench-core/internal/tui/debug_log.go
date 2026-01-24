package tui

import (
	"encoding/json"
	"os"
	"time"
)

func logDebug(hypothesisId, location, message string, data map[string]interface{}) {
	f, err := os.OpenFile("/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "debug-run",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	enc := json.NewEncoder(f)
	_ = enc.Encode(payload)
}
