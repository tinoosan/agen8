package user

import (
	"fmt"
	"strings"
)

// ID is the typed account identifier owned by the user domain.
type ID struct {
	value string
}

func NewID(value string) (ID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ID{}, fmt.Errorf("user id is required")
	}
	return ID{value: value}, nil
}

func (id ID) String() string {
	return id.value
}

func (id ID) IsZero() bool {
	return strings.TrimSpace(id.value) == ""
}
