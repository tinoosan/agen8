package store

import (
	"errors"
	"time"
)

// ResultWriter is used by the tool runner to persist call outputs.
type ResultWriter interface {
	PutCall(callID string, responseJSON []byte) error
	PutArtifact(callID, artifactPath, mediaType string, content []byte) error
}

// ResultReader is used by VFS to serve reads.
type ResultReader interface {
	GetCallResponseJSON(callID string) ([]byte, error)
	GetArtifact(callID, artifactPath string) ([]byte, string, error)
}

// ResultLister is used by VFS to serve listings.
type ResultLister interface {
	ListCallIDs() ([]string, error)
	ListArtifacts(callID string) ([]ArtifactMeta, error)
}

// ResultsView is the minimal store contract needed by the /results VFS resource.
type ResultsView interface {
	ResultReader
	ResultLister
}

// ResultsStore is the host-side storage interface for tool call outputs.
type ResultsStore interface {
	ResultWriter
	ResultReader
	ResultLister
}

var (
	// ErrResultsNotFound indicates a requested callId or artifact is not present in the store.
	ErrResultsNotFound = errors.Join(ErrNotFound, errors.New("results not found"))
)

// ArtifactMeta describes one artifact stored under a call.
type ArtifactMeta struct {
	Path      string
	MediaType string
	Size      int64
	ModTime   time.Time
}
