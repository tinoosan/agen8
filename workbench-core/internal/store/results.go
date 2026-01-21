package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/internal/validate"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
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
//
// This is the backing store for the virtual VFS mount "/results".
//
// # Goal
//
// The agent interacts with results via VFS paths:
//   - fs.list("/results") -> call IDs (directories)
//   - fs.read("/results/<callId>/response.json") -> the ToolResponse JSON bytes
//   - fs.read("/results/<callId>/<artifactPath>") -> artifact bytes (if present)
//
// With ResultsStore, the host can keep "/results" virtual (in-memory, DB-backed, remote, etc)
// while preserving the exact same agent-visible VFS interface.
//
// # Search note
//
// Builtin tools operate on disk-backed roots (typically the workspace) and do not see virtual
// mounts like "/results". Searching virtual mounts requires store-backed search APIs (future work).
type ResultsStore interface {
	ResultWriter
	ResultReader
	ResultLister
}

var (
	// ErrResultsNotFound indicates a requested callId or artifact is not present in the store.
	//
	// This is joined with ErrNotFound so callers can check either:
	//   - errors.Is(err, store.ErrResultsNotFound)
	//   - errors.Is(err, store.ErrNotFound)
	ErrResultsNotFound = errors.Join(ErrNotFound, errors.New("results not found"))
)

// ArtifactMeta describes one artifact stored under a call.
type ArtifactMeta struct {
	Path      string
	MediaType string
	Size      int64
	ModTime   time.Time
}

// InMemoryResultsStore is the simplest ResultsStore implementation.
//
// It is fast and deterministic, making it ideal for:
// - demos
// - tests
// - early iterations before choosing a durable store
//
// It is NOT durable; everything is lost when the process exits.
type InMemoryResultsStore struct {
	mu    sync.RWMutex
	calls map[string]*inMemoryCall
}

type inMemoryCall struct {
	responseJSON []byte
	responseTime time.Time
	artifacts    map[string]inMemoryArtifact
}

type inMemoryArtifact struct {
	b         []byte
	mediaType string
	modTime   time.Time
}

// NewInMemoryResultsStore creates an empty in-memory ResultsStore.
func NewInMemoryResultsStore() *InMemoryResultsStore {
	return &InMemoryResultsStore{
		calls: make(map[string]*inMemoryCall),
	}
}

func (s *InMemoryResultsStore) PutCall(callID string, responseJSON []byte) error {
	if s == nil {
		return fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return err
	}
	if responseJSON == nil {
		return fmt.Errorf("responseJSON is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.calls[callID]
	if c == nil {
		c = &inMemoryCall{artifacts: make(map[string]inMemoryArtifact)}
		s.calls[callID] = c
	}
	c.responseJSON = append([]byte(nil), responseJSON...)
	c.responseTime = time.Now().UTC()
	return nil
}

func (s *InMemoryResultsStore) GetCallResponseJSON(callID string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil || c.responseJSON == nil {
		return nil, ErrResultsNotFound
	}
	return append([]byte(nil), c.responseJSON...), nil
}

func (s *InMemoryResultsStore) ListCallIDs() ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, 0, len(s.calls))
	for id := range s.calls {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *InMemoryResultsStore) PutArtifact(callID, artifactPath, mediaType string, content []byte) error {
	if s == nil {
		return fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return err
	}
	artifactPath = strings.TrimSpace(artifactPath)
	if err := validate.NonEmpty("artifactPath", artifactPath); err != nil {
		return err
	}
	if err := validate.NonEmpty("mediaType", mediaType); err != nil {
		return err
	}
	if content == nil {
		return fmt.Errorf("content is required")
	}
	clean, err := vfsutil.CleanRelPath(artifactPath)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.calls[callID]
	if c == nil {
		c = &inMemoryCall{artifacts: make(map[string]inMemoryArtifact)}
		s.calls[callID] = c
	}
	if c.artifacts == nil {
		c.artifacts = make(map[string]inMemoryArtifact)
	}
	c.artifacts[clean] = inMemoryArtifact{
		b:         append([]byte(nil), content...),
		mediaType: mediaType,
		modTime:   time.Now().UTC(),
	}
	return nil
}

func (s *InMemoryResultsStore) GetArtifact(callID, artifactPath string) ([]byte, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, "", err
	}
	artifactPath = strings.TrimSpace(artifactPath)
	if err := validate.NonEmpty("artifactPath", artifactPath); err != nil {
		return nil, "", err
	}
	clean, err := vfsutil.CleanRelPath(artifactPath)
	if err != nil {
		return nil, "", err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil || c.artifacts == nil {
		return nil, "", ErrResultsNotFound
	}
	a, ok := c.artifacts[clean]
	if !ok {
		return nil, "", ErrResultsNotFound
	}
	return append([]byte(nil), a.b...), a.mediaType, nil
}

func (s *InMemoryResultsStore) ListArtifacts(callID string) ([]ArtifactMeta, error) {
	if s == nil {
		return nil, fmt.Errorf("results store is nil")
	}
	callID = strings.TrimSpace(callID)
	if err := validate.NonEmpty("callID", callID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.calls[callID]
	if c == nil {
		return nil, ErrResultsNotFound
	}

	out := make([]ArtifactMeta, 0, len(c.artifacts))
	for p, a := range c.artifacts {
		out = append(out, ArtifactMeta{
			Path:      p,
			MediaType: a.mediaType,
			Size:      int64(len(a.b)),
			ModTime:   a.modTime,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
