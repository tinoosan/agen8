package types

import "time"

type RunStatus string

type Run struct {
	RunId, Goal           string
	Status                RunStatus
	StartedAt, FinishedAt time.Time
	MaxBytesForContext    int
	Err                 error
}


