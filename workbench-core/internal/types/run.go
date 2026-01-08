package types

import "time"

type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusDone    RunStatus = "done"
	StatusFailed  RunStatus = "failed"
)

var statusName = map[RunStatus]string{
	StatusRunning: "running",
	StatusDone:    "done",
	StatusFailed:  "failed",
}

type Run struct {
	RunId              string    `json:"runId'`
	Goal               string    `json:"goal"`
	Status             RunStatus `json:"status"`
	StartedAt          time.Time `json:"startedAt"`
	FinishedAt         time.Time `json:"finishedAt"`
	MaxBytesForContext int       `json:"maxBytesForContext"`
	Err                error     `json:"error"`
}
