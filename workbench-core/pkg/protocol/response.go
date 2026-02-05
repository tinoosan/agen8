package protocol

// ThreadCreateResult is the result for thread.create.
type ThreadCreateResult struct {
	Thread Thread `json:"thread"`
}

// ThreadGetResult is the result for thread.get.
type ThreadGetResult struct {
	Thread Thread `json:"thread"`
}

// TurnCreateResult is the result for turn.create.
type TurnCreateResult struct {
	Turn Turn `json:"turn"`
}

// TurnCancelResult is the result for turn.cancel.
type TurnCancelResult struct {
	Turn Turn `json:"turn"`
}

// ItemListResult is the result for item.list.
type ItemListResult struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
