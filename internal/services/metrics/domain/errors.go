package domain

import "fmt"

// ErrInvalidInput indicates a business-rule validation failure.
type ErrInvalidInput struct {
	Field   string
	Message string
}

func (e *ErrInvalidInput) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

// ErrServiceUnavailable indicates a required dependency is not configured.
type ErrServiceUnavailable struct {
	Service string
	Reason  string
}

func (e *ErrServiceUnavailable) Error() string {
	return fmt.Sprintf("%s unavailable: %s", e.Service, e.Reason)
}
