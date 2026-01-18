package validate

import (
	"fmt"
	"strings"
)

// NonEmpty returns an error if s is empty after trimming whitespace.
func NonEmpty(name, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

// NonNil returns an error if v is nil.
func NonNil[T any](name string, v *T) error {
	if v == nil {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

// Positive returns an error if n is not > 0.
func Positive(name string, n int) error {
	if n <= 0 {
		return fmt.Errorf("%s must be > 0", name)
	}
	return nil
}

