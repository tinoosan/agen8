package domain

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }
