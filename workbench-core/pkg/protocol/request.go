package protocol

// Request method names.
const (
	MethodThreadCreate = "thread.create"
	MethodThreadGet    = "thread.get"
	MethodTurnCreate   = "turn.create"
	MethodTurnCancel   = "turn.cancel"
	MethodItemList     = "item.list"
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
