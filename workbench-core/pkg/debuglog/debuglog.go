package debuglog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
)

const (
	sessionID = "debug-session"

	// EnvDebugLogPath overrides the debug log file path.
	EnvDebugLogPath = "WORKBENCH_DEBUG_LOG_PATH"
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
	f, err := OpenLogFile()
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

// ResolveDebugLogPath returns the debug log file path.
func ResolveDebugLogPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvDebugLogPath)); override != "" {
		expanded, err := expandTilde(override)
		if err != nil {
			return "", err
		}
		return filepath.Clean(expanded), nil
	}

	dataDir, err := config.ResolveDataDir("", false)
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "debug.log"), nil
}

// OpenLogFile opens the debug log file for append.
func OpenLogFile() (*os.File, error) {
	path, err := ResolveDebugLogPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func expandTilde(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", os.ErrInvalid
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", os.ErrInvalid
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
}
