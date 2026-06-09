package app

import (
	"context"
	"time"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
)

type Clock interface {
	Now() time.Time
}

type HostnameResolver func() (string, error)

type ProjectReferenceChecker interface {
	HasProjectsForLocation(ctx context.Context, locationID locationdomain.ID) (bool, error)
}

type Transport interface {
	Probe(ctx context.Context, location locationdomain.Location) (ProbeResult, error)
	ListDir(ctx context.Context, location locationdomain.Location, path string) ([]DirEntry, error)
}

type ProbeResult struct {
	Reachable    bool
	FileBrowsing bool
	Exec         bool
	Codex        bool
	Claude       bool
	Status       locationdomain.ProbeStatus
	FailureCode  locationdomain.FailureCode
	Message      string
	ProbedAt     time.Time
}

type DirEntry struct {
	Name string
	Path string
	Type locationdomain.DirEntryType
	Size int64
}
