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
	"github.com/tinoosan/workbench-core/internal/validate"
)

// CreateSession creates and persists a new session.
//
// Sessions are stored under:
//
//	data/sessions/<sessionId>/session.json
//
// The session history file is created lazily by the history store/sink.
func CreateSession(cfg config.Config, title string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	s := types.NewSession(title)
	return s, SaveSession(cfg, s)
}

// SaveSession persists a session's session.json file.
func SaveSession(cfg config.Config, s types.Session) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := validate.NonEmpty("sessionId", s.SessionID); err != nil {
		return err
	}
	targetPath := fsutil.GetSessionFilePath(cfg.DataDir, s.SessionID)
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
func LoadSession(cfg config.Config, sessionID string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Session{}, err
	}
	targetPath := fsutil.GetSessionFilePath(cfg.DataDir, sessionID)
	b, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.Session{}, fmt.Errorf("session.json file %s does not exist: %w", targetPath, errors.Join(ErrNotFound, err))
		}
		return types.Session{}, fmt.Errorf("error reading session.json file %s: %w", targetPath, err)
	}
	var s types.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return types.Session{}, fmt.Errorf("error unmarshalling json file %s: %w", targetPath, err)
	}
	if err := validate.NonEmpty("sessionId", s.SessionID); err != nil {
		return types.Session{}, fmt.Errorf("invalid session.json: missing sessionId: %w", ErrInvalid)
	}
	return s, nil
}

// AddRunToSession appends runId to the session index (if not already present)
// and updates CurrentRunID.
func AddRunToSession(cfg config.Config, sessionID, runID string) (types.Session, error) {
	if err := cfg.Validate(); err != nil {
		return types.Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Session{}, err
	}
	if err := validate.NonEmpty("runId", runID); err != nil {
		return types.Session{}, err
	}
	s, err := LoadSession(cfg, sessionID)
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
	return s, SaveSession(cfg, s)
}

// ListSessionIDs returns all session IDs currently on disk, sorted ascending.
func ListSessionIDs(cfg config.Config) ([]string, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	base := fsutil.GetSessionsDir(cfg.DataDir)
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
