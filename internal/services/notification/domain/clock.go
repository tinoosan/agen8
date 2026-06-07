package domain

import "time"

// Clock abstracts "now" so derivation and reconciliation are deterministic
// under test. Production wiring uses SystemClock.
type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }
