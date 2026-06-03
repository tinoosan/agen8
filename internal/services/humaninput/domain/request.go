package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type RequestID string
type ToolCallID string
type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
	StatusAborted   Status = "aborted"
)

type Request struct {
	ID             RequestID       `json:"id"`
	ToolCallID     ToolCallID      `json:"toolCallId"`
	ToolName       string          `json:"toolName"`
	IdempotencyKey string          `json:"idempotencyKey"`
	ProjectID      string          `json:"projectId"`
	SpaceID        string          `json:"spaceId"`
	AskerMemberID  string          `json:"askerMemberId"`
	ChannelID      string          `json:"channelId"`
	Declaration    Declaration     `json:"declaration"`
	Status         Status          `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	TerminalReason string          `json:"terminalReason,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	ExpiresAt      time.Time       `json:"expiresAt"`
	ResolvedAt     *time.Time      `json:"resolvedAt,omitempty"`
	Version        int64           `json:"version"`
}

type Filter struct {
	ProjectID string
	SpaceID   string
	ChannelID string
	MemberID  string
	Kind      PrimitiveKind
	Limit     int
	Offset    int
}

type ResolveMutation struct {
	ID               RequestID
	ExpectedVersion  int64
	Status           Status
	Result           json.RawMessage
	ResolverUserID   string
	ResolverMemberID string
	TerminalReason   string
	ResolvedAt       time.Time
}

type ExpireBatch struct {
	Requests []Request
	Count    int
}

func NewPending(input PendingInput) (Request, error) {
	req := Request{
		ID:             RequestID(strings.TrimSpace(string(input.ID))),
		ToolCallID:     ToolCallID(strings.TrimSpace(string(input.ToolCallID))),
		ToolName:       strings.TrimSpace(input.ToolName),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		ProjectID:      strings.TrimSpace(input.ProjectID),
		SpaceID:        strings.TrimSpace(input.SpaceID),
		AskerMemberID:  strings.TrimSpace(input.AskerMemberID),
		ChannelID:      strings.TrimSpace(input.ChannelID),
		Declaration:    input.Declaration,
		Status:         StatusPending,
		Result:         json.RawMessage(`{}`),
		CreatedAt:      input.CreatedAt.UTC(),
		ExpiresAt:      input.ExpiresAt.UTC(),
		Version:        1,
	}
	if err := req.Validate(); err != nil {
		return Request{}, err
	}
	return req, nil
}

type PendingInput struct {
	ID             RequestID
	ToolCallID     ToolCallID
	ToolName       string
	IdempotencyKey string
	ProjectID      string
	SpaceID        string
	AskerMemberID  string
	ChannelID      string
	Declaration    Declaration
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

func (r Request) Validate() error {
	if strings.TrimSpace(string(r.ID)) == "" {
		return fmt.Errorf("human input request id is required")
	}
	if strings.TrimSpace(string(r.ToolCallID)) == "" {
		return fmt.Errorf("human input tool call id is required")
	}
	if strings.TrimSpace(r.ToolName) == "" {
		return fmt.Errorf("human input tool name is required")
	}
	if strings.TrimSpace(r.IdempotencyKey) == "" {
		return fmt.Errorf("human input idempotency key is required")
	}
	if strings.TrimSpace(r.ProjectID) == "" {
		return fmt.Errorf("human input project id is required")
	}
	if strings.TrimSpace(r.SpaceID) == "" {
		return fmt.Errorf("human input space id is required")
	}
	if strings.TrimSpace(r.AskerMemberID) == "" {
		return fmt.Errorf("human input asker member id is required")
	}
	if strings.TrimSpace(r.ChannelID) == "" {
		return fmt.Errorf("human input channel id is required")
	}
	if err := r.Declaration.Validate(); err != nil {
		return err
	}
	switch r.Status {
	case StatusPending, StatusAnswered, StatusCancelled, StatusExpired, StatusAborted:
	default:
		return fmt.Errorf("human input status %q is invalid", strings.TrimSpace(string(r.Status)))
	}
	if r.CreatedAt.IsZero() {
		return fmt.Errorf("human input created_at is required")
	}
	if r.ExpiresAt.IsZero() {
		return fmt.Errorf("human input expires_at is required")
	}
	if !r.ExpiresAt.After(r.CreatedAt) {
		return fmt.Errorf("human input expires_at must be after created_at")
	}
	if len(r.Result) > 0 && !json.Valid(r.Result) {
		return fmt.Errorf("human input result must be valid JSON")
	}
	if r.Version <= 0 {
		return fmt.Errorf("human input version must be positive")
	}
	return nil
}

func IsTerminal(status Status) bool {
	switch status {
	case StatusAnswered, StatusCancelled, StatusExpired, StatusAborted:
		return true
	default:
		return false
	}
}
