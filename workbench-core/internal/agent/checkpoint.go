package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/types"
)

// AgentCheckpoint captures enough agent-loop state to resume an in-flight turn.
//
// This is intentionally host-facing state (durable checkpoint), not provenance.
type AgentCheckpoint struct {
	// Version is reserved for future migrations.
	Version int `json:"version,omitempty"`

	// UserMessage is the external user message that started the turn (not a HostOpResponse).
	UserMessage string `json:"userMessage,omitempty"`

	// NextStep is the next agent loop step to execute (1-based).
	NextStep int `json:"nextStep"`

	// Messages is the full conversation transcript as used by the agent loop.
	Messages []types.LLMMessage `json:"messages"`

	// LastOp is the last parsed model JSON object (for debugging).
	LastOp string `json:"lastOp,omitempty"`

	// LastResponseID is the provider response ID used for Responses API chaining.
	LastResponseID string `json:"lastResponseId,omitempty"`

	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

func (c AgentCheckpoint) Validate() error {
	if c.NextStep < 1 {
		return fmt.Errorf("nextStep must be >= 1")
	}
	if len(c.Messages) == 0 {
		return fmt.Errorf("messages must be non-empty")
	}
	return nil
}

// LoadAgentCheckpoint loads a checkpoint from disk.
//
// If the file does not exist, it returns (nil, nil).
func LoadAgentCheckpoint(path string) (*AgentCheckpoint, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cp AgentCheckpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, err
	}
	if err := cp.Validate(); err != nil {
		return nil, err
	}
	return &cp, nil
}

// SaveAgentCheckpoint atomically writes a checkpoint to disk.
func SaveAgentCheckpoint(path string, cp AgentCheckpoint) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("checkpoint path is required")
	}
	if err := cp.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	cp.UpdatedAt = &now

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := jsonutil.MarshalPretty(cp)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, b, 0o644)
}

// ClearAgentCheckpoint removes a checkpoint file (best-effort).
func ClearAgentCheckpoint(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

