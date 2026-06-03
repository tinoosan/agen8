package types

// Error describes a turn/item failure payload embedded in turn/item content.
// It is not the repository's general-purpose domain error type.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// DomainError is an internal semantic error value used within application and
// domain code. Code values belong to the domain layer and do not share the
// JSON-RPC numeric code space used by package protocol.
type DomainError struct {
	Code    string
	Message string
	Cause   error
}

func (e *DomainError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" && e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *DomainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
