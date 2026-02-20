package timeutil

import (
	"strings"
	"time"
)

// IsSet returns true if the pointer is non-nil and the referenced time is not zero.
func IsSet(t *time.Time) bool {
	return t != nil && !t.IsZero()
}

// FormatRFC3339Nano formats a time pointer as RFC3339Nano, returning an empty string when unset.
func FormatRFC3339Nano(t *time.Time) string {
	if !IsSet(t) {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseRFC3339Nano parses a RFC3339Nano timestamp string, returning zero time on failure.
func ParseRFC3339Nano(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// OrNow returns the referenced time in UTC or the current time when unset.
func OrNow(t *time.Time) time.Time {
	if IsSet(t) {
		return t.UTC()
	}
	return time.Now().UTC()
}

// FirstNonZero returns the first non-nil/non-zero time from the provided list.
func FirstNonZero(times ...*time.Time) time.Time {
	for _, t := range times {
		if IsSet(t) {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Since returns the time elapsed since t.
func Since(t time.Time) time.Duration {
	return time.Since(t)
}
