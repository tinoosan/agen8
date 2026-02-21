package protocol

import (
	"encoding/json"
	"time"
)

// ItemID uniquely identifies an item.
type ItemID string

// ItemType identifies the shape of Item.Content.
type ItemType string

const (
	ItemTypeUserMessage   ItemType = "user_message"
	ItemTypeAgentMessage  ItemType = "agent_message"
	ItemTypeToolExecution ItemType = "tool_execution"
	ItemTypeReasoning     ItemType = "reasoning"
)

// ItemStatus represents the lifecycle state of an item.
type ItemStatus string

const (
	ItemStatusStarted   ItemStatus = "started"
	ItemStatusStreaming ItemStatus = "streaming"
	ItemStatusCompleted ItemStatus = "completed"
	ItemStatusFailed    ItemStatus = "failed"
	ItemStatusCanceled  ItemStatus = "canceled"
)

// Item is an atomic unit of work within a turn.
//
// Content is a type-specific JSON payload; use the *Content types in this file.
type Item struct {
	ID        ItemID          `json:"id"`
	TurnID    TurnID          `json:"turnId"`
	RunID     RunID           `json:"runId,omitempty"`
	Type      ItemType        `json:"type"`
	Status    ItemStatus      `json:"status"`
	CreatedAt time.Time       `json:"createdAt,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Error     *Error          `json:"error,omitempty"`
}

// DecodeContent unmarshals i.Content into v.
func (i Item) DecodeContent(v any) error {
	if len(i.Content) == 0 {
		return nil
	}
	return json.Unmarshal(i.Content, v)
}

// SetContent marshals v into i.Content.
func (i *Item) SetContent(v any) error {
	if i == nil {
		return nil
	}
	if v == nil {
		i.Content = nil
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	i.Content = b
	return nil
}

// AttachmentRef references an attachment (for example an image or file) associated with a message.
type AttachmentRef struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	URI       string `json:"uri,omitempty"`
}

// ArtifactRef references a generated artifact (for example a file path or resource URI).
type ArtifactRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	URI  string `json:"uri,omitempty"`
}

// UserMessageContent is the content payload for ItemTypeUserMessage.
type UserMessageContent struct {
	Text        string          `json:"text"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

// AgentMessageContent is the content payload for ItemTypeAgentMessage.
type AgentMessageContent struct {
	Text      string        `json:"text"`
	IsPartial bool          `json:"isPartial,omitempty"`
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
}

// ToolExecutionContent is the content payload for ItemTypeToolExecution.
type ToolExecutionContent struct {
	ToolName string          `json:"toolName"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
	Ok       bool            `json:"ok,omitempty"`
}

// ReasoningContent is the content payload for ItemTypeReasoning.
type ReasoningContent struct {
	Summary string `json:"summary,omitempty"`
	Step    int    `json:"step,omitempty"`
}

// ItemDeltaParams is the notification params for item.delta.
type ItemDeltaParams struct {
	ItemID ItemID    `json:"itemId"`
	Delta  ItemDelta `json:"delta"`
}

// ItemDelta is an incremental update to an item (streaming).
type ItemDelta struct {
	TextDelta      string `json:"textDelta,omitempty"`
	ReasoningDelta string `json:"reasoningDelta,omitempty"`
}
