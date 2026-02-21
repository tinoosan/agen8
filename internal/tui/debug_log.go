package tui

import (
	"encoding/json"
	"time"

	"github.com/tinoosan/agen8/pkg/debuglog"
)

func logDebug(hypothesisId, location, message string, data map[string]interface{}) {
	f, err := debuglog.OpenLogFile()
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
