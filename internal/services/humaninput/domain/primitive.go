package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type PrimitiveKind string

const (
	PrimitiveQuestions     PrimitiveKind = "questions"
	PrimitiveApproveReject PrimitiveKind = "approve_reject"
	PrimitiveConfirm       PrimitiveKind = "confirm"
	PrimitiveForm          PrimitiveKind = "form"
)

type Declaration struct {
	Kind    PrimitiveKind   `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func (d Declaration) Validate() error {
	switch d.Kind {
	case PrimitiveQuestions, PrimitiveApproveReject, PrimitiveConfirm, PrimitiveForm:
		// valid
	default:
		return fmt.Errorf("human input kind %q is invalid", strings.TrimSpace(string(d.Kind)))
	}
	if len(d.Payload) == 0 || !json.Valid(d.Payload) {
		return fmt.Errorf("human input payload must be valid JSON")
	}
	return nil
}

// PendingRequest is the input to a human-input awaiter. Identity
// (SpaceID, MemberID) names the asker; ChannelID names the panel
// delivery target. All three are required by the awaiter — leaving
// any one empty is a wiring bug, not a runtime fallback.
type PendingRequest struct {
	ToolCallID     string      `json:"toolCallId"`
	ToolName       string      `json:"toolName"`
	IdempotencyKey string      `json:"idempotencyKey,omitempty"`
	Declaration    Declaration `json:"declaration"`
	ProjectID      string      `json:"projectId,omitempty"`
	SpaceID        string      `json:"spaceId,omitempty"`
	MemberID       string      `json:"memberId,omitempty"`
	ChannelID      string      `json:"channelId,omitempty"`
	CreatedAtRFC   string      `json:"createdAt,omitempty"`
}

type Awaiter func(ctx context.Context, req PendingRequest) (json.RawMessage, error)
