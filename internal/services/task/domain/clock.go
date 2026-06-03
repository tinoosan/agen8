package domain

import "time"

// Clock returns the current time. Injected so service-layer code that
// needs to stamp CompletedAt/UpdatedAt can be tested with a fixed
// time, and so the domain transitions stay pure (they receive the
// time as a parameter rather than calling time.Now() themselves).
type Clock interface {
	Now() time.Time
}

// SystemClock is the production wall-clock implementation.
type SystemClock struct{}

// Now returns time.Now() in the system's location. Callers that need
// UTC normalize at the point of use; transition methods on Task
// already do so.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock returns a single configured time on every call. Use in
// tests to assert exact timestamps without tolerance windows.
type FixedClock struct{ T time.Time }

// Now returns the fixed time.
func (c FixedClock) Now() time.Time { return c.T }
