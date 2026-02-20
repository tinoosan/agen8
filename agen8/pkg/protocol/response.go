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

type SoulDoc struct {
	Content   string `json:"content"`
	Version   int    `json:"version"`
	Checksum  string `json:"checksum"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
	Locked    bool   `json:"locked,omitempty"`
}

type SoulGetResult struct {
	Soul SoulDoc `json:"soul"`
}

type SoulUpdateResult struct {
	Soul SoulDoc `json:"soul"`
}

type SoulAuditEvent struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp,omitempty"`
	ActorLayer     string `json:"actorLayer"`
	Action         string `json:"action"`
	Reason         string `json:"reason,omitempty"`
	VersionBefore  int    `json:"versionBefore"`
	VersionAfter   int    `json:"versionAfter"`
	ChecksumBefore string `json:"checksumBefore,omitempty"`
	ChecksumAfter  string `json:"checksumAfter,omitempty"`
}

type SoulHistoryResult struct {
	Events     []SoulAuditEvent `json:"events"`
	NextCursor string           `json:"nextCursor,omitempty"`
}
