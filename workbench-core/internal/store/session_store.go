package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/types"
)

// CreateSession creates and persists a new session.
//
// Sessions are stored under:
//
//	data/sessions/<sessionId>/session.json
//
// The session history file is created lazily by the history store/sink.
func CreateSession(title string) (types.Session, error) {
	s := types.NewSession(title)
	return s, SaveSession(s)
}

// SaveSession persists a session's session.json file.
func SaveSession(s types.Session) error {
	if strings.TrimSpace(s.SessionID) == "" {
		return fmt.Errorf("sessionId is required")
	}
	targetPath := fsutil.GetSessionFilePath(config.DataDir, s.SessionID)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	b, err := jsonutil.MarshalPretty(s)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(targetPath, b, 0644)
}

// LoadSession reads session.json for a session ID.
func LoadSession(sessionID string) (types.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return types.Session{}, fmt.Errorf("sessionId is required")
	}
	targetPath := fsutil.GetSessionFilePath(config.DataDir, sessionID)
	b, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.Session{}, fmt.Errorf("session.json file %s does not exist: %w", targetPath, err)
		}
		return types.Session{}, fmt.Errorf("error reading session.json file %s: %w", targetPath, err)
	}
	var s types.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return types.Session{}, fmt.Errorf("error unmarshalling json file %s: %w", targetPath, err)
	}
	if strings.TrimSpace(s.SessionID) == "" {
		return types.Session{}, fmt.Errorf("invalid session.json: missing sessionId")
	}
	return s, nil
}

// AddRunToSession appends runId to the session index (if not already present)
// and updates CurrentRunID.
func AddRunToSession(sessionID, runID string) (types.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" {
		return types.Session{}, fmt.Errorf("sessionId is required")
	}
	if runID == "" {
		return types.Session{}, fmt.Errorf("runId is required")
	}
	s, err := LoadSession(sessionID)
	if err != nil {
		return types.Session{}, err
	}
	seen := false
	for _, existing := range s.Runs {
		if existing == runID {
			seen = true
			break
		}
	}
	if !seen {
		s.Runs = append(s.Runs, runID)
	}
	s.CurrentRunID = runID
	return s, SaveSession(s)
}

// ListSessionIDs returns all session IDs currently on disk, sorted ascending.
func ListSessionIDs() ([]string, error) {
	base := fsutil.GetSessionsDir(config.DataDir)
	des, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(des))
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		out = append(out, de.Name())
	}
	sort.Strings(out)
	return out, nil
}
