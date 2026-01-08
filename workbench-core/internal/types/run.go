package types

import (
	"time"

	"github.com/google/uuid"
)

type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusDone    RunStatus = "done"
	StatusFailed  RunStatus = "failed"
)

type Run struct {
	RunId              string     `json:"runId"`
	Goal               string     `json:"goal"`
	Status             RunStatus  `json:"status"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
	MaxBytesForContext int        `json:"maxBytesForContext"`
	Error              *string    `json:"error,omitempty"`
}

func NewRun(goal string, maxBytesForContext int) Run {
	runId := "run-" + uuid.NewString()
	now := time.Now()
	return Run{
		RunId:              runId,
		Goal:               goal,
		Status:             StatusRunning,
		StartedAt:          &now,
		MaxBytesForContext: maxBytesForContext,
		Error:              nil,
	}

}
