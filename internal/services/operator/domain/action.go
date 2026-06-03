// Package domain defines the Operator Action aggregate root and its lifecycle.
//
// An Operator Action represents asynchronous real-world work delegation:
// "I need you to DO something." The operator goes away, does work in the real world,
// and comes back with structured outcome data. This is fundamentally different from
// Escalation (which is a synchronous decision gate).
//
// Status lifecycle:
//
//	pending -> acknowledged -> in_progress -> completed|blocked|canceled
//	                                      -> pending_verification (when requiresVerification=true)
//	pending_verification -> completed (agent accepts) | in_progress (agent requests changes)
package domain

import (
	"fmt"
	"strings"
	"time"
)

// OperatorActionID is the unique identifier for an operator action.
type OperatorActionID string

// OAStatus tracks the lifecycle of an operator action.
type OAStatus string

const (
	OAStatusPending             OAStatus = "pending"
	OAStatusAcknowledged        OAStatus = "acknowledged"
	OAStatusInProgress          OAStatus = "in_progress"
	OAStatusPendingVerification OAStatus = "pending_verification"
	OAStatusCompleted           OAStatus = "completed"
	OAStatusBlocked             OAStatus = "blocked"
	OAStatusCanceled            OAStatus = "canceled"
)

// ValidOAStatuses is the exhaustive set of valid operator action statuses.
var ValidOAStatuses = []OAStatus{
	OAStatusPending,
	OAStatusAcknowledged,
	OAStatusInProgress,
	OAStatusPendingVerification,
	OAStatusCompleted,
	OAStatusBlocked,
	OAStatusCanceled,
}

// ValidateOAStatus returns an error if the status is not a valid OAStatus.
func ValidateOAStatus(s OAStatus) error {
	switch s {
	case OAStatusPending, OAStatusAcknowledged, OAStatusInProgress,
		OAStatusPendingVerification, OAStatusCompleted,
		OAStatusBlocked, OAStatusCanceled:
		return nil
	default:
		return fmt.Errorf("invalid operator action status %q", s)
	}
}

// OutcomeStatus is the result of the operator's real-world work.
// Separate from OAStatus -- an action can be "completed" with outcome "partial".
type OutcomeStatus string

const (
	OutcomeCompleted OutcomeStatus = "completed"
	OutcomePartial   OutcomeStatus = "partial"
	OutcomeFailed    OutcomeStatus = "failed"
)

// ValidOutcomeStatuses is the exhaustive set of valid outcome statuses.
var ValidOutcomeStatuses = []OutcomeStatus{
	OutcomeCompleted,
	OutcomePartial,
	OutcomeFailed,
}

// ValidateOutcomeStatus returns an error if the outcome status is not valid.
func ValidateOutcomeStatus(s OutcomeStatus) error {
	switch s {
	case OutcomeCompleted, OutcomePartial, OutcomeFailed:
		return nil
	default:
		return fmt.Errorf("invalid outcome status %q: must be one of completed, partial, failed", s)
	}
}

// OASource identifies who created the operator action.
type OASource = Source

const (
	OASourceMember   OASource = SourceMember
	OASourceOperator OASource = SourceOperator
)

// ValidateOASource returns an error if the source is not valid.
func ValidateOASource(s OASource) error {
	if err := ValidateSource(Source(s)); err != nil {
		return fmt.Errorf("invalid operator action source %q: must be one of member, operator", s)
	}
	return nil
}

// ProgressNote is a timestamped update from the operator while work is in_progress.
type ProgressNote struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// Attachment is evidence linked to an OA -- either a locally-stored file or an external URL reference.
type Attachment struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // "file" or "url"
	Filename    string    `json:"filename,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	SizeBytes   int64     `json:"sizeBytes,omitempty"`
	StoragePath string    `json:"storagePath,omitempty"`
	URL         string    `json:"url,omitempty"`
	Label       string    `json:"label,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Comment is a bidirectional message between operator and member on an in-progress OA.
type Comment struct {
	Author    string    `json:"author"` // "operator", "member:<label>"
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// OperatorAction is the aggregate root for real-world operator work.
type OperatorAction struct {
	ID                   OperatorActionID  `json:"id"`
	ProjectID            string            `json:"projectId"`
	SpaceID              string            `json:"spaceId,omitempty"`
	TaskRef              string            `json:"taskRef,omitempty"`
	KeyResultRef         string            `json:"keyResultRef,omitempty"`
	MissionRef           string            `json:"missionRef,omitempty"`
	RunID                string            `json:"runId,omitempty"`
	Blocking             bool              `json:"blocking"`
	Source               OASource          `json:"source"`
	MemberID             string            `json:"memberId,omitempty"`
	EscalationRef        string            `json:"escalationRef,omitempty"`
	Category             Category          `json:"category"`
	Urgency              Urgency           `json:"urgency"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	RequiresVerification bool              `json:"requiresVerification"`
	Status               OAStatus          `json:"status"`
	OutcomeStatus        OutcomeStatus     `json:"outcomeStatus,omitempty"`
	OutcomeSummary       string            `json:"outcomeSummary,omitempty"`
	OutcomePairs         map[string]string `json:"outcomePairs,omitempty"`
	Attachments          []Attachment      `json:"attachments,omitempty"`
	ProgressNotes        []ProgressNote    `json:"progressNotes,omitempty"`
	Comments             []Comment         `json:"comments,omitempty"`
	Deadline             *time.Time        `json:"deadline,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	CreatedAt            time.Time         `json:"createdAt"`
	AcknowledgedAt       *time.Time        `json:"acknowledgedAt,omitempty"`
	StartedAt            *time.Time        `json:"startedAt,omitempty"`
	CompletedAt          *time.Time        `json:"completedAt,omitempty"`
	VerifiedAt           *time.Time        `json:"verifiedAt,omitempty"`
}

// CreateParams holds all fields needed to create a new OperatorAction.
type CreateParams struct {
	ID                   OperatorActionID
	ProjectID            string
	SpaceID              string
	TaskRef              string
	KeyResultRef         string
	MissionRef           string
	RunID                string
	Blocking             bool
	Source               OASource
	MemberID             string
	EscalationRef        string
	Category             Category
	Urgency              Urgency
	Title                string
	Description          string
	RequiresVerification bool
	Deadline             *time.Time
	Metadata             map[string]string
}

// Validate checks that all required fields are present and valid.
func (p CreateParams) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("operator action ID is required")
	}
	if p.ProjectID == "" {
		return fmt.Errorf("operator action projectId is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("operator action title is required")
	}
	if err := ValidateOASource(p.Source); err != nil {
		return err
	}
	if err := ValidateCategory(p.Category); err != nil {
		return err
	}
	return ValidateUrgency(p.Urgency)
}

// NewOperatorAction creates a new OperatorAction from validated parameters.
// Returns an error if any required field is missing or invalid.
func NewOperatorAction(p CreateParams, now time.Time) (OperatorAction, error) {
	if err := p.Validate(); err != nil {
		return OperatorAction{}, err
	}
	return OperatorAction{
		ID:                   p.ID,
		ProjectID:            p.ProjectID,
		SpaceID:              p.SpaceID,
		TaskRef:              p.TaskRef,
		KeyResultRef:         p.KeyResultRef,
		MissionRef:           p.MissionRef,
		RunID:                p.RunID,
		Blocking:             p.Blocking,
		Source:               p.Source,
		MemberID:             p.MemberID,
		EscalationRef:        p.EscalationRef,
		Category:             p.Category,
		Urgency:              p.Urgency,
		Title:                p.Title,
		Description:          p.Description,
		RequiresVerification: p.RequiresVerification,
		Status:               OAStatusPending,
		Deadline:             p.Deadline,
		Metadata:             p.Metadata,
		CreatedAt:            now,
	}, nil
}

// validTransitions defines the allowed status transitions.
// Terminal states (completed, canceled) have no outgoing transitions.
var validTransitions = map[OAStatus]map[OAStatus]bool{
	OAStatusPending: {
		OAStatusAcknowledged: true,
		OAStatusCanceled:     true,
	},
	OAStatusAcknowledged: {
		OAStatusInProgress: true,
		OAStatusCanceled:   true,
	},
	OAStatusInProgress: {
		OAStatusCompleted:           true,
		OAStatusPendingVerification: true,
		OAStatusBlocked:             true,
		OAStatusCanceled:            true,
	},
	OAStatusPendingVerification: {
		OAStatusCompleted:  true,
		OAStatusInProgress: true,
		OAStatusCanceled:   true,
	},
	OAStatusBlocked: {
		OAStatusInProgress: true,
		OAStatusCanceled:   true,
	},
	// Terminal states: no outgoing transitions.
	OAStatusCompleted: {},
	OAStatusCanceled:  {},
}

// CanTransitionTo returns true if the transition from the current status to the target is valid.
func (oa *OperatorAction) CanTransitionTo(target OAStatus) bool {
	if oa == nil {
		return false
	}
	allowed, exists := validTransitions[oa.Status]
	if !exists {
		return false
	}
	return allowed[target]
}

// Acknowledge transitions pending -> acknowledged. Auto-ack on view.
func (oa *OperatorAction) Acknowledge(now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if !oa.CanTransitionTo(OAStatusAcknowledged) {
		return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusAcknowledged)
	}
	oa.Status = OAStatusAcknowledged
	oa.AcknowledgedAt = &now
	return nil
}

// Start transitions acknowledged -> in_progress. Operator clicks "Start".
func (oa *OperatorAction) Start(now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if !oa.CanTransitionTo(OAStatusInProgress) {
		return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusInProgress)
	}
	oa.Status = OAStatusInProgress
	oa.StartedAt = &now
	return nil
}

// CompleteOutcome holds the operator's reported outcome data.
type CompleteOutcome struct {
	OutcomeStatus  OutcomeStatus
	OutcomeSummary string
	OutcomePairs   map[string]string
}

// Validate checks that the outcome has required fields.
func (o CompleteOutcome) Validate() error {
	if err := ValidateOutcomeStatus(o.OutcomeStatus); err != nil {
		return err
	}
	if o.OutcomeSummary == "" {
		return fmt.Errorf("outcome summary is required for completion")
	}
	return nil
}

// Complete transitions in_progress -> completed or pending_verification.
// If requiresVerification is true, goes to pending_verification instead of completed.
// Completion REQUIRES outcome data (outcome_status + outcome_summary).
func (oa *OperatorAction) Complete(outcome CompleteOutcome, now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if err := outcome.Validate(); err != nil {
		return err
	}

	if oa.RequiresVerification {
		if !oa.CanTransitionTo(OAStatusPendingVerification) {
			return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusPendingVerification)
		}
		oa.Status = OAStatusPendingVerification
	} else {
		if !oa.CanTransitionTo(OAStatusCompleted) {
			return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusCompleted)
		}
		oa.Status = OAStatusCompleted
		oa.CompletedAt = &now
	}

	oa.OutcomeStatus = outcome.OutcomeStatus
	oa.OutcomeSummary = outcome.OutcomeSummary
	oa.OutcomePairs = outcome.OutcomePairs
	return nil
}

// Verify is called by the agent after operator reports outcome on a requiresVerification=true action.
// accepted=true -> completed. accepted=false -> in_progress (with feedback as progress note).
func (oa *OperatorAction) Verify(accepted bool, feedback string, now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if oa.Status != OAStatusPendingVerification {
		return fmt.Errorf("Verify requires status %q, got %q", OAStatusPendingVerification, oa.Status)
	}
	if accepted {
		if !oa.CanTransitionTo(OAStatusCompleted) {
			return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusCompleted)
		}
		oa.Status = OAStatusCompleted
		oa.CompletedAt = &now
		oa.VerifiedAt = &now
	} else {
		if feedback == "" {
			return fmt.Errorf("feedback is required when requesting changes")
		}
		if !oa.CanTransitionTo(OAStatusInProgress) {
			return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusInProgress)
		}
		oa.Status = OAStatusInProgress
		// Clear outcome data so operator provides fresh outcome on re-completion.
		oa.OutcomeStatus = ""
		oa.OutcomeSummary = ""
		oa.OutcomePairs = nil
		// Append feedback as a progress note.
		oa.ProgressNotes = append(oa.ProgressNotes, ProgressNote{
			Text:      fmt.Sprintf("[Verification feedback] %s", feedback),
			CreatedAt: now,
		})
	}
	return nil
}

// Block transitions in_progress -> blocked. Operator can't proceed.
func (oa *OperatorAction) Block(reason string, now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if reason == "" {
		return fmt.Errorf("block reason is required")
	}
	if !oa.CanTransitionTo(OAStatusBlocked) {
		return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusBlocked)
	}
	oa.Status = OAStatusBlocked
	oa.ProgressNotes = append(oa.ProgressNotes, ProgressNote{
		Text:      fmt.Sprintf("[Blocked] %s", reason),
		CreatedAt: now,
	})
	return nil
}

// Unblock transitions blocked -> in_progress. Blocker resolved.
func (oa *OperatorAction) Unblock(now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if !oa.CanTransitionTo(OAStatusInProgress) {
		return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusInProgress)
	}
	oa.Status = OAStatusInProgress
	oa.ProgressNotes = append(oa.ProgressNotes, ProgressNote{
		Text:      "[Unblocked] Blocker resolved, resuming work",
		CreatedAt: now,
	})
	return nil
}

// Cancel transitions any non-terminal state to canceled.
func (oa *OperatorAction) Cancel(now time.Time) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if !oa.CanTransitionTo(OAStatusCanceled) {
		return fmt.Errorf("cannot transition from %q to %q", oa.Status, OAStatusCanceled)
	}
	oa.Status = OAStatusCanceled
	oa.CompletedAt = &now
	return nil
}

// AddProgressNote appends a progress note. Only valid when in_progress or blocked.
func (oa *OperatorAction) AddProgressNote(note ProgressNote) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if strings.TrimSpace(note.Text) == "" {
		return fmt.Errorf("progress note text is required")
	}
	if note.CreatedAt.IsZero() {
		return fmt.Errorf("progress note timestamp is required")
	}
	if oa.Status != OAStatusInProgress && oa.Status != OAStatusBlocked {
		return fmt.Errorf("progress notes can only be added when status is in_progress or blocked, got %q", oa.Status)
	}
	oa.ProgressNotes = append(oa.ProgressNotes, note)
	return nil
}

// AddComment appends a comment. Only valid when in_progress, blocked, or pending_verification.
func (oa *OperatorAction) AddComment(comment Comment) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if strings.TrimSpace(comment.Author) == "" {
		return fmt.Errorf("comment author is required")
	}
	if strings.TrimSpace(comment.Text) == "" {
		return fmt.Errorf("comment text is required")
	}
	if comment.CreatedAt.IsZero() {
		return fmt.Errorf("comment timestamp is required")
	}
	if oa.Status != OAStatusInProgress && oa.Status != OAStatusBlocked && oa.Status != OAStatusPendingVerification {
		return fmt.Errorf("comments can only be added when status is in_progress, blocked, or pending_verification, got %q", oa.Status)
	}
	oa.Comments = append(oa.Comments, comment)
	return nil
}

// AddAttachment appends an attachment. Only valid when in_progress, blocked, or pending_verification.
func (oa *OperatorAction) AddAttachment(attachment Attachment) error {
	if oa == nil {
		return fmt.Errorf("operator action is nil")
	}
	if attachment.ID == "" {
		return fmt.Errorf("attachment ID is required")
	}
	if attachment.Kind != "file" && attachment.Kind != "url" {
		return fmt.Errorf("attachment kind must be \"file\" or \"url\", got %q", attachment.Kind)
	}
	if attachment.CreatedAt.IsZero() {
		return fmt.Errorf("attachment timestamp is required")
	}
	if oa.Status != OAStatusInProgress && oa.Status != OAStatusBlocked && oa.Status != OAStatusPendingVerification {
		return fmt.Errorf("attachments can only be added when status is in_progress, blocked, or pending_verification, got %q", oa.Status)
	}
	oa.Attachments = append(oa.Attachments, attachment)
	return nil
}

// IsTerminal returns true if the action is in a terminal state (completed or canceled).
func (oa *OperatorAction) IsTerminal() bool {
	if oa == nil {
		return false
	}
	return oa.Status == OAStatusCompleted || oa.Status == OAStatusCanceled
}
