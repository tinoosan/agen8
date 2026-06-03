package app

import "time"

// Clock supplies timestamps for app-layer orchestration.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the process wall clock.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock returns a stable timestamp for tests.
type FixedClock struct{ T time.Time }

// Now returns the configured fixed time.
func (c FixedClock) Now() time.Time { return c.T }
