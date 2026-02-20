package store

import (
	"fmt"
	"strconv"
	"strings"
)

// OffsetCursor is an opaque, stable position token encoded as a base-10 int64 byte offset.
//
// This is intentionally a string at the module boundary. Callers should treat it as opaque.
// Specific stores may choose to interpret it as a byte offset (e.g., disk JSONL stores).
type OffsetCursor string

// OffsetCursorFromInt64 encodes a byte offset cursor as an opaque token.
func OffsetCursorFromInt64(offset int64) OffsetCursor {
	if offset < 0 {
		offset = 0
	}
	return OffsetCursor(strconv.FormatInt(offset, 10))
}

// OffsetCursorToInt64 decodes an OffsetCursor into a byte offset.
//
// If the cursor is empty, it decodes to 0.
func OffsetCursorToInt64(c OffsetCursor) (int64, error) {
	s := strings.TrimSpace(string(c))
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor: %w", ErrInvalid)
	}
	return n, nil
}
