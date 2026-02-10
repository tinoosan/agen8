package protocol

// Request method names.
const (
	MethodThreadCreate      = "thread.create"
	MethodThreadGet         = "thread.get"
	MethodTurnCreate        = "turn.create"
	MethodTurnCancel        = "turn.cancel"
	MethodItemList          = "item.list"
	MethodTaskList          = "task.list"
	MethodTaskClaim         = "task.claim"
	MethodTaskCreate        = "task.create"
	MethodTaskComplete      = "task.complete"
	MethodSessionStart      = "session.start"
	MethodSessionList       = "session.list"
	MethodSessionRename     = "session.rename"
	MethodAgentList         = "agent.list"
	MethodAgentStart        = "agent.start"
	MethodAgentPause        = "agent.pause"
	MethodAgentResume       = "agent.resume"
	MethodSessionPause      = "session.pause"
	MethodSessionResume     = "session.resume"
	MethodSessionGetTotals  = "session.getTotals"
	MethodActivityList      = "activity.list"
	MethodTeamGetStatus     = "team.getStatus"
	MethodTeamGetManifest   = "team.getManifest"
	MethodPlanGet           = "plan.get"
	MethodModelList         = "model.list"
	MethodControlSetModel   = "control.setModel"
	MethodControlSetProfile = "control.setProfile"
	MethodArtifactList      = "artifact.list"
	MethodArtifactSearch    = "artifact.search"
	MethodArtifactGet       = "artifact.get"
)

// ThreadCreateParams are the params for thread.create.
type ThreadCreateParams struct {
	Title       string `json:"title,omitempty"`
	ActiveModel string `json:"activeModel,omitempty"`
}

// ThreadGetParams are the params for thread.get.
type ThreadGetParams struct {
	ThreadID ThreadID `json:"threadId"`
}

// TurnCreateParams are the params for turn.create.
type TurnCreateParams struct {
	ThreadID ThreadID `json:"threadId"`

	// Input is an optional user message that begins the turn.
	Input *UserMessageContent `json:"input,omitempty"`
}

// TurnCancelParams are the params for turn.cancel.
type TurnCancelParams struct {
	TurnID TurnID `json:"turnId"`
}

// ItemListParams are the params for item.list.
type ItemListParams struct {
	ThreadID ThreadID `json:"threadId,omitempty"`
	TurnID   TurnID   `json:"turnId,omitempty"`

	// Cursor is an opaque pagination cursor.
	Cursor string `json:"cursor,omitempty"`
	// Limit caps the number of items returned.
	Limit int `json:"limit,omitempty"`
}

// ArtifactListParams are the params for artifact.list.
type ArtifactListParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`

	DayBucket string `json:"dayBucket,omitempty"`
	Role      string `json:"role,omitempty"`
	TaskKind  string `json:"taskKind,omitempty"`
	TaskID    string `json:"taskId,omitempty"`

	Limit int `json:"limit,omitempty"`
}

// ArtifactSearchParams are the params for artifact.search.
type ArtifactSearchParams struct {
	ThreadID ThreadID `json:"threadId"`
	TeamID   string   `json:"teamId,omitempty"`
	Query    string   `json:"query"`
	ScopeKey string   `json:"scopeKey,omitempty"`

	DayBucket string `json:"dayBucket,omitempty"`
	Role      string `json:"role,omitempty"`
	TaskKind  string `json:"taskKind,omitempty"`
	TaskID    string `json:"taskId,omitempty"`

	Limit int `json:"limit,omitempty"`
}

// ArtifactGetParams are the params for artifact.get.
type ArtifactGetParams struct {
	ThreadID   ThreadID `json:"threadId"`
	TeamID     string   `json:"teamId,omitempty"`
	ArtifactID string   `json:"artifactId,omitempty"`
	VPath      string   `json:"vpath,omitempty"`
	MaxBytes   int      `json:"maxBytes,omitempty"`
}
