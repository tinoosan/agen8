package protocol

import (
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// F23: Operator Action Lifecycle (new two-primitive model)
// These methods operate on the new opaction domain (op_actions table).
// ────────────────────────────────────────────────────────────────────────────

const (
	MethodOpActionCreate      = "opAction.create"
	MethodOpActionGet         = "opAction.get"
	MethodOpActionList        = "opAction.list"
	MethodOpActionListPending = "opAction.listPending"
	MethodOpActionAcknowledge = "opAction.acknowledge"
	MethodOpActionStart       = "opAction.start"
	MethodOpActionComplete    = "opAction.complete"
	MethodOpActionVerify      = "opAction.verify"
	MethodOpActionBlock       = "opAction.block"
	MethodOpActionUnblock     = "opAction.unblock"
	MethodOpActionCancel      = "opAction.cancel"
	MethodOpActionAddNote     = "opAction.addNote"
	MethodOpActionAddComment  = "opAction.addComment"
	MethodOpActionCountStatus = "opAction.countStatus"
)

// OpActionView is the wire-format read model for the new operator action entity.
type OpActionView struct {
	ID                   string                   `json:"id"`
	ProjectID            string                   `json:"projectId"`
	SpaceID              string                   `json:"spaceId,omitempty"`
	TaskRef              string                   `json:"taskRef,omitempty"`
	KeyResultRef         string                   `json:"keyResultRef,omitempty"`
	MissionRef           string                   `json:"missionRef,omitempty"`
	RunID                string                   `json:"runId,omitempty"`
	Blocking             bool                     `json:"blocking"`
	Source               string                   `json:"source"`
	MemberID             string                   `json:"memberId,omitempty"`
	EscalationRef        string                   `json:"escalationRef,omitempty"`
	Category             string                   `json:"category"`
	Urgency              string                   `json:"urgency"`
	Title                string                   `json:"title"`
	Description          string                   `json:"description"`
	RequiresVerification bool                     `json:"requiresVerification"`
	Status               string                   `json:"status"`
	OutcomeStatus        string                   `json:"outcomeStatus,omitempty"`
	OutcomeSummary       string                   `json:"outcomeSummary,omitempty"`
	OutcomePairs         map[string]string        `json:"outcomePairs,omitempty"`
	Attachments          []OpActionAttachmentView `json:"attachments,omitempty"`
	ProgressNotes        []OpActionNoteView       `json:"progressNotes,omitempty"`
	Comments             []OpActionCommentView    `json:"comments,omitempty"`
	Deadline             *time.Time               `json:"deadline,omitempty"`
	Metadata             map[string]string        `json:"metadata,omitempty"`
	CreatedAt            time.Time                `json:"createdAt"`
	AcknowledgedAt       *time.Time               `json:"acknowledgedAt,omitempty"`
	StartedAt            *time.Time               `json:"startedAt,omitempty"`
	CompletedAt          *time.Time               `json:"completedAt,omitempty"`
	VerifiedAt           *time.Time               `json:"verifiedAt,omitempty"`
}

// OpActionAttachmentView is the wire-format for an attachment.
type OpActionAttachmentView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Filename    string    `json:"filename,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	SizeBytes   int64     `json:"sizeBytes,omitempty"`
	URL         string    `json:"url,omitempty"`
	Label       string    `json:"label,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// OpActionNoteView is the wire-format for a progress note.
type OpActionNoteView struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// OpActionCommentView is the wire-format for a comment.
type OpActionCommentView struct {
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// -- opAction.create --

type OpActionCreateParams struct {
	ProjectID            string            `json:"projectId"`
	SpaceID              string            `json:"spaceId,omitempty"`
	TaskRef              string            `json:"taskRef,omitempty"`
	KeyResultRef         string            `json:"keyResultRef,omitempty"`
	MissionRef           string            `json:"missionRef,omitempty"`
	RunID                string            `json:"runId,omitempty"`
	Blocking             bool              `json:"blocking"`
	Source               string            `json:"source"`
	MemberID             string            `json:"memberId,omitempty"`
	EscalationRef        string            `json:"escalationRef,omitempty"`
	Category             string            `json:"category"`
	Urgency              string            `json:"urgency"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	RequiresVerification bool              `json:"requiresVerification"`
	Deadline             *time.Time        `json:"deadline,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

type OpActionCreateResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.get --

type OpActionGetParams struct {
	ActionID string `json:"actionId"`
}

type OpActionGetResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.list --

type OpActionListParams struct {
	ProjectID string   `json:"projectId"`
	Status    []string `json:"status,omitempty"`
	Urgency   []string `json:"urgency,omitempty"`
	Category  []string `json:"category,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type OpActionListResult struct {
	OpActions []OpActionView `json:"opActions"`
}

// -- opAction.listPending --

type OpActionListPendingParams struct {
	ProjectID string `json:"projectId"`
}

type OpActionListPendingResult struct {
	OpActions []OpActionView `json:"opActions"`
}

// -- opAction.acknowledge --

type OpActionAcknowledgeParams struct {
	ActionID string `json:"actionId"`
}

type OpActionAcknowledgeResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.start --

type OpActionStartParams struct {
	ActionID string `json:"actionId"`
}

type OpActionStartResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.complete --

type OpActionCompleteParams struct {
	ActionID       string            `json:"actionId"`
	OutcomeStatus  string            `json:"outcomeStatus"`
	OutcomeSummary string            `json:"outcomeSummary"`
	OutcomePairs   map[string]string `json:"outcomePairs,omitempty"`
}

type OpActionCompleteResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.verify --

type OpActionVerifyParams struct {
	ActionID string `json:"actionId"`
	Accepted bool   `json:"accepted"`
	Feedback string `json:"feedback,omitempty"`
	Author   string `json:"author,omitempty"`
}

type OpActionVerifyResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.block --

type OpActionBlockParams struct {
	ActionID string `json:"actionId"`
	Reason   string `json:"reason"`
}

type OpActionBlockResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.unblock --

type OpActionUnblockParams struct {
	ActionID string `json:"actionId"`
}

type OpActionUnblockResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.cancel --

type OpActionCancelParams struct {
	ActionID string `json:"actionId"`
}

type OpActionCancelResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.addNote --

type OpActionAddNoteParams struct {
	ActionID string `json:"actionId"`
	Text     string `json:"text"`
}

type OpActionAddNoteResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- opAction.addComment --

type OpActionAddCommentParams struct {
	ActionID string `json:"actionId"`
	Author   string `json:"author"`
	Text     string `json:"text"`
}

type OpActionAddCommentResult struct {
	OpAction OpActionView `json:"opAction"`
}

// -- attachment.upload --

type AttachmentUploadParams struct {
	ActionID    string `json:"actionId"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType,omitempty"`
	BytesB64    string `json:"bytesB64"`
}

type AttachmentUploadResult struct {
	Attachment OpActionAttachmentView `json:"attachment"`
	OpAction   OpActionView           `json:"opAction"`
}

// -- attachment.addURL --

type AttachmentAddURLParams struct {
	ActionID string `json:"actionId"`
	URL      string `json:"url"`
	Label    string `json:"label,omitempty"`
}

type AttachmentAddURLResult struct {
	Attachment OpActionAttachmentView `json:"attachment"`
	OpAction   OpActionView           `json:"opAction"`
}

// -- opAction.countStatus --

type OpActionCountStatusParams struct {
	ProjectID string `json:"projectId"`
}

type OpActionCountStatusResult struct {
	Counts map[string]int `json:"counts"`
}
