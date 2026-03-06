package protocol

import "time"

type MessageListParams struct {
	ThreadID ThreadID `json:"threadId,omitempty"`
	View     string   `json:"view,omitempty"`  // inbox|outbox
	Scope    string   `json:"scope,omitempty"` // team|run (default: auto)
	TeamID   string   `json:"teamId,omitempty"`
	RunID    string   `json:"runId,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	Statuses []string `json:"statuses,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Offset   int      `json:"offset,omitempty"`
}

type MessageGetParams struct {
	ThreadID  ThreadID `json:"threadId,omitempty"`
	TeamID    string   `json:"teamId,omitempty"`
	MessageID string   `json:"messageId"`
}

type MailMessage struct {
	MessageID         string     `json:"messageId"`
	ThreadID          ThreadID   `json:"threadId"`
	RunID             RunID      `json:"runId,omitempty"`
	SourceTeamID      string     `json:"sourceTeamId,omitempty"`
	DestinationTeamID string     `json:"destinationTeamId,omitempty"`
	TeamID            string     `json:"teamId,omitempty"`
	Channel           string     `json:"channel"`
	Kind              string     `json:"kind"`
	Status            string     `json:"status"`
	Subject           string     `json:"subject,omitempty"`
	Summary           string     `json:"summary,omitempty"`
	BodyPreview       string     `json:"bodyPreview,omitempty"`
	Error             string     `json:"error,omitempty"`
	TaskID            string     `json:"taskId,omitempty"`
	TaskStatus        string     `json:"taskStatus,omitempty"`
	ReadOnly          bool       `json:"readOnly,omitempty"`
	CanClaim          bool       `json:"canClaim,omitempty"`
	CanComplete       bool       `json:"canComplete,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	ProcessedAt       *time.Time `json:"processedAt,omitempty"`
	Task              *Task      `json:"task,omitempty"`
}

type MessageListResult struct {
	Messages   []MailMessage `json:"messages"`
	TotalCount int           `json:"totalCount,omitempty"`
}

type MessageGetResult struct {
	Message MailMessage `json:"message"`
}
