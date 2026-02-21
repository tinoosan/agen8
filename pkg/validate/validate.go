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
