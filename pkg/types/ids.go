package types

// Canonical typed identifiers for domain entities.
//
// These remain string-backed so JSON/database encoding stays wire-compatible
// while giving the compiler enough information to catch cross-assignment.
type (
	ChannelID      ID
	TurnID         ID
	RunID          ID
	EventID        ID
	MessageID      ID
	AgentMessageID ID
	IntentID       ID
	CorrelationID  ID
	CausationID    ID
	LocationID     ID
	ProjectID      ID
	RoleID         ID
)

// ID is the shared underlying representation for typed identifiers.
type ID string

func (id ID) String() string {
	return string(id)
}
