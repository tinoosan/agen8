package app

import "fmt"

type ValidationError struct {
	message string
}

func (e ValidationError) Error() string {
	return e.message
}

func validationError(format string, args ...any) error {
	return ValidationError{message: fmt.Sprintf(format, args...)}
}
