package vfs

import (
	"fmt"
	"strings"
)

// ReadOnlyResource provides standard errors for write attempts.
type ReadOnlyResource struct {
	Name string
}

func (r ReadOnlyResource) Write(_ string, _ []byte) error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = "resource"
	}
	return fmt.Errorf("%s is read-only", name)
}

func (r ReadOnlyResource) Append(_ string, _ []byte) error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = "resource"
	}
	return fmt.Errorf("%s is read-only", name)
}
