package validate

import (
	"fmt"
	"strings"
)

func NonEmpty(name, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func NonNil[T any](name string, v *T) error {
	if v == nil {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func Positive(name string, n int) error {
	if n <= 0 {
		return fmt.Errorf("%s must be > 0", name)
	}
	return nil
}
